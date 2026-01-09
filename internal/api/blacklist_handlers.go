package api

import (
	"bufio"
	"context"
	"database/sql"
	"log"
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

	// Use ClickHouse if available and has data
	if s.ch != nil {
		ctx := context.Background()
		entries, total, err := s.ch.SearchBlacklist(ctx, search, limit, offset)
		if err != nil {
			log.Printf("[Blacklist] ClickHouse error: %v, falling back to PostgreSQL", err)
		} else if total > 0 || (page == 1 && search == "") {
			// Return ClickHouse data if we have results, or if it's the first page with no search
			// (to handle the case where ClickHouse is empty but PostgreSQL has data, we check PostgreSQL below)
			var blacklist []fiber.Map
			for _, entry := range entries {
				blacklist = append(blacklist, fiber.Map{
					"id":         entry.ID,
					"email":      entry.Email,
					"reason":     entry.Reason,
					"created_at": entry.CreatedAt,
				})
			}

			// If ClickHouse returned 0 total on first page, check PostgreSQL as well
			if total == 0 && page == 1 && search == "" {
				var pgTotal int
				s.db.QueryRow(`SELECT COUNT(*) FROM blacklist`).Scan(&pgTotal)
				if pgTotal > 0 {
					log.Printf("[Blacklist] ClickHouse empty but PostgreSQL has %d entries, using PostgreSQL", pgTotal)
					goto usePostgres
				}
			}

			return c.JSON(fiber.Map{
				"data":  blacklist,
				"total": total,
				"page":  page,
				"limit": limit,
			})
		}
	}

usePostgres:

	// Fallback to PostgreSQL
	var total int
	if search != "" {
		s.db.QueryRow(`SELECT COUNT(*) FROM blacklist WHERE email LIKE $1`, "%"+search+"%").Scan(&total)
	} else {
		s.db.QueryRow(`SELECT COUNT(*) FROM blacklist`).Scan(&total)
	}

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

	// Insert into ClickHouse if available
	if s.ch != nil {
		ctx := context.Background()
		_, _, err := s.ch.InsertBlacklistBatch(ctx, []string{email}, req.Reason)
		if err != nil {
			log.Printf("[Blacklist] ClickHouse insert error: %v", err)
		}
	}

	// ALWAYS also insert into PostgreSQL as backup
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

	// Get the email from PostgreSQL first
	var email string
	s.db.QueryRow(`SELECT email FROM blacklist WHERE id = $1`, id).Scan(&email)

	// Delete from ClickHouse if available
	if s.ch != nil {
		ctx := context.Background()
		err := s.ch.DeleteFromBlacklist(ctx, id)
		if err != nil {
			log.Printf("[Blacklist] ClickHouse delete error: %v", err)
		}
	}

	// ALWAYS also delete from PostgreSQL
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

// clearBlacklist removes all entries from the blacklist
func (s *Server) clearBlacklist(c *fiber.Ctx) error {
	var count int

	// Clear ClickHouse if available
	if s.ch != nil {
		ctx := context.Background()
		total, err := s.ch.GetBlacklistCount(ctx)
		if err == nil {
			count = int(total)
			err = s.ch.ClearBlacklist(ctx)
			if err != nil {
				log.Printf("[Blacklist] ClickHouse clear error: %v, falling back to PostgreSQL", err)
			} else {
				// Also clear from PostgreSQL for consistency
				s.db.Exec(`DELETE FROM blacklist`)
				s.db.Exec(`DELETE FROM blacklist_import_jobs`)
				s.db.Exec(`UPDATE emails SET valid = true WHERE bounced = false AND unsubscribed = false`)

				return c.JSON(fiber.Map{
					"message": "Blacklist limpa com sucesso",
					"deleted": count,
				})
			}
		}
	}

	// Fallback to PostgreSQL
	s.db.QueryRow(`SELECT COUNT(*) FROM blacklist`).Scan(&count)

	_, err := s.db.Exec(`DELETE FROM blacklist`)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Falha ao limpar blacklist"})
	}

	s.db.Exec(`DELETE FROM blacklist_import_jobs`)
	s.db.Exec(`UPDATE emails SET valid = true WHERE bounced = false AND unsubscribed = false`)

	return c.JSON(fiber.Map{
		"message": "Blacklist limpa com sucesso",
		"deleted": count,
	})
}

// importBlacklist imports emails from file (simple text file, one email per line)
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

	// Collect emails first
	var emails []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		email := strings.ToLower(strings.TrimSpace(scanner.Text()))
		if email != "" && emailRegex.MatchString(email) {
			emails = append(emails, email)
		}
	}

	if len(emails) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "No valid emails found in file"})
	}

	imported := 0
	duplicates := 0
	chInserted := 0

	// Insert into ClickHouse if available (primary storage for bulk data)
	if s.ch != nil {
		ctx := context.Background()
		ins, _, err := s.ch.InsertBlacklistBatch(ctx, emails, reason)
		if err != nil {
			log.Printf("[Blacklist] ClickHouse bulk insert error: %v", err)
		} else {
			chInserted = int(ins)
			log.Printf("[Blacklist] ClickHouse: inserted %d emails", chInserted)
		}
	}

	// ALWAYS also insert into PostgreSQL as backup
	tx, _ := s.db.Begin()

	for _, email := range emails {
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

	// Use ClickHouse count if higher (PostgreSQL may have had some duplicates already)
	if chInserted > imported {
		imported = chInserted
	}

	return c.JSON(fiber.Map{
		"message":    "Blacklist imported",
		"imported":   imported,
		"duplicates": duplicates,
	})
}
