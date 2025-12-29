package api

import (
	"bufio"
	"database/sql"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type BlacklistRequest struct {
	Email  string `json:"email"`
	Reason string `json:"reason"`
}

// listBlacklist returns all blacklisted emails
func (s *Server) listBlacklist(c *fiber.Ctx) error {
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 50)
	search := c.Query("search", "")
	offset := (page - 1) * limit

	// Get total count
	var total int
	if search != "" {
		s.db.QueryRow(`SELECT COUNT(*) FROM blacklist WHERE email LIKE $1`, "%"+search+"%").Scan(&total)
	} else {
		s.db.QueryRow(`SELECT COUNT(*) FROM blacklist`).Scan(&total)
	}

	// Build query
	var rows *sql.Rows
	var err error
	if search != "" {
		rows, err = s.db.Query(`
			SELECT id, email, reason, created_at
			FROM blacklist
			WHERE email LIKE $1
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3
		`, "%"+search+"%", limit, offset)
	} else {
		rows, err = s.db.Query(`
			SELECT id, email, reason, created_at
			FROM blacklist
			ORDER BY created_at DESC
			LIMIT $1 OFFSET $2
		`, limit, offset)
	}

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch blacklist"})
	}
	defer rows.Close()

	var blacklist []fiber.Map
	for rows.Next() {
		var id, email string
		var reason *string
		var createdAt time.Time

		rows.Scan(&id, &email, &reason, &createdAt)
		blacklist = append(blacklist, fiber.Map{
			"id":         id,
			"email":      email,
			"reason":     reason,
			"created_at": createdAt,
		})
	}

	return c.JSON(fiber.Map{
		"data":  blacklist,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// addToBlacklist adds an email to blacklist
func (s *Server) addToBlacklist(c *fiber.Ctx) error {
	var req BlacklistRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.Email == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Email is required"})
	}

	if req.Reason == "" {
		req.Reason = "manual"
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	id := uuid.New().String()

	_, err := s.db.Exec(`
		INSERT INTO blacklist (id, email, reason)
		VALUES ($1, $2, $3)
		ON CONFLICT (email) DO UPDATE SET reason = $3
	`, id, email, req.Reason)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to add to blacklist"})
	}

	// Mark email as invalid in all lists
	s.db.Exec(`UPDATE emails SET valid = false WHERE LOWER(email) = $1`, email)

	return c.Status(201).JSON(fiber.Map{
		"message": "Email added to blacklist",
		"id":      id,
	})
}

// removeFromBlacklist removes an email from blacklist
func (s *Server) removeFromBlacklist(c *fiber.Ctx) error {
	id := c.Params("id")

	// Get email before deleting
	var email string
	s.db.QueryRow(`SELECT email FROM blacklist WHERE id = $1`, id).Scan(&email)

	result, err := s.db.Exec(`DELETE FROM blacklist WHERE id = $1`, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to remove from blacklist"})
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "Blacklist entry not found"})
	}

	// Mark email as valid in all lists (if not bounced/unsubscribed)
	if email != "" {
		s.db.Exec(`UPDATE emails SET valid = true WHERE LOWER(email) = $1 AND bounced = false AND unsubscribed = false`, strings.ToLower(email))
	}

	return c.JSON(fiber.Map{"message": "Email removed from blacklist"})
}

// importBlacklist imports emails from file
func (s *Server) importBlacklist(c *fiber.Ctx) error {
	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "No file uploaded"})
	}

	reason := c.FormValue("reason", "import")

	f, err := file.Open()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to open file"})
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	imported := 0
	duplicates := 0

	tx, _ := s.db.Begin()

	for scanner.Scan() {
		email := strings.ToLower(strings.TrimSpace(scanner.Text()))
		if email == "" || !emailRegex.MatchString(email) {
			continue
		}

		id := uuid.New().String()
		result, err := tx.Exec(`
			INSERT INTO blacklist (id, email, reason)
			VALUES ($1, $2, $3)
			ON CONFLICT (email) DO NOTHING
		`, id, email, reason)

		if err == nil {
			rows, _ := result.RowsAffected()
			if rows > 0 {
				imported++
			} else {
				duplicates++
			}
		}
	}

	tx.Commit()

	// Update emails table
	s.db.Exec(`
		UPDATE emails SET valid = false
		WHERE LOWER(email) IN (SELECT email FROM blacklist)
	`)

	return c.JSON(fiber.Map{
		"message":    "Blacklist imported",
		"imported":   imported,
		"duplicates": duplicates,
	})
}
