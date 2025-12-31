package api

import (
	"bufio"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type EmailListRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

// listEmailLists returns all email lists
func (s *Server) listEmailLists(c *fiber.Ctx) error {
	rows, err := s.db.Query(`
		SELECT id, name, description, total_emails, valid_emails,
		       invalid_emails, status, created_at, updated_at
		FROM email_lists
		ORDER BY created_at DESC
	`)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch email lists"})
	}
	defer rows.Close()

	var lists []fiber.Map
	for rows.Next() {
		var id, name, status string
		var description *string
		var totalEmails, validEmails, invalidEmails int
		var createdAt, updatedAt time.Time

		err := rows.Scan(&id, &name, &description, &totalEmails, &validEmails,
			&invalidEmails, &status, &createdAt, &updatedAt)
		if err != nil {
			continue
		}

		lists = append(lists, fiber.Map{
			"id":             id,
			"name":           name,
			"description":    description,
			"total_emails":   totalEmails,
			"valid_emails":   validEmails,
			"invalid_emails": invalidEmails,
			"status":         status,
			"created_at":     createdAt,
			"updated_at":     updatedAt,
		})
	}

	return c.JSON(fiber.Map{
		"data":  lists,
		"total": len(lists),
	})
}

// createEmailList creates a new email list
func (s *Server) createEmailList(c *fiber.Ctx) error {
	var req EmailListRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Name is required"})
	}

	id := uuid.New().String()

	_, err := s.db.Exec(`
		INSERT INTO email_lists (id, name, description, status)
		VALUES ($1, $2, $3, 'ready')
	`, id, req.Name, req.Description)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create email list"})
	}

	return c.Status(201).JSON(fiber.Map{
		"message": "Email list created",
		"id":      id,
	})
}

// getEmailList returns a single email list
func (s *Server) getEmailList(c *fiber.Ctx) error {
	id := c.Params("id")

	var name, status string
	var description *string
	var totalEmails, validEmails, invalidEmails int
	var createdAt, updatedAt time.Time

	err := s.db.QueryRow(`
		SELECT name, description, total_emails, valid_emails,
		       invalid_emails, status, created_at, updated_at
		FROM email_lists WHERE id = $1
	`, id).Scan(&name, &description, &totalEmails, &validEmails,
		&invalidEmails, &status, &createdAt, &updatedAt)

	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Email list not found"})
	}

	return c.JSON(fiber.Map{
		"id":             id,
		"name":           name,
		"description":    description,
		"total_emails":   totalEmails,
		"valid_emails":   validEmails,
		"invalid_emails": invalidEmails,
		"status":         status,
		"created_at":     createdAt,
		"updated_at":     updatedAt,
	})
}

// updateEmailList updates an email list
func (s *Server) updateEmailList(c *fiber.Ctx) error {
	id := c.Params("id")

	var req EmailListRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	result, err := s.db.Exec(`
		UPDATE email_lists SET name = $1, description = $2 WHERE id = $3
	`, req.Name, req.Description, id)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update email list"})
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "Email list not found"})
	}

	return c.JSON(fiber.Map{"message": "Email list updated"})
}

// deleteEmailList deletes an email list
func (s *Server) deleteEmailList(c *fiber.Ctx) error {
	id := c.Params("id")

	result, err := s.db.Exec(`DELETE FROM email_lists WHERE id = $1`, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete email list"})
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "Email list not found"})
	}

	return c.JSON(fiber.Map{"message": "Email list deleted"})
}

// uploadEmails handles CSV/TXT file upload
func (s *Server) uploadEmails(c *fiber.Ctx) error {
	id := c.Params("id")

	// Check if list exists
	var exists bool
	s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM email_lists WHERE id = $1)`, id).Scan(&exists)
	if !exists {
		return c.Status(404).JSON(fiber.Map{"error": "Email list not found"})
	}

	// Get file
	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "No file uploaded"})
	}

	// Get column mappings
	emailCol := c.FormValue("email_column", "0")
	nameCol := c.FormValue("name_column", "1")
	delimiter := c.FormValue("delimiter", ",")
	hasHeader := c.FormValue("has_header", "true") == "true"

	// Open file
	f, err := file.Open()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to open file"})
	}
	defer f.Close()

	// Update list status
	s.db.Exec(`UPDATE email_lists SET status = 'processing' WHERE id = $1`, id)

	// Process file with larger buffer for big files
	scanner := bufio.NewScanner(f)
	// Increase buffer size for large files (1MB buffer)
	const maxCapacity = 1024 * 1024
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	lineNum := 0
	validCount := 0
	invalidCount := 0
	duplicateCount := 0

	log.Printf("Starting email import for list %s from file %s", id, file.Filename)

	// Get existing emails for duplicate check
	existingEmails := make(map[string]bool)
	rows, _ := s.db.Query(`SELECT email FROM emails WHERE list_id = $1`, id)
	for rows.Next() {
		var email string
		rows.Scan(&email)
		existingEmails[strings.ToLower(email)] = true
	}
	rows.Close()

	// Also check blacklist
	blacklisted := make(map[string]bool)
	blRows, _ := s.db.Query(`SELECT email FROM blacklist`)
	for blRows.Next() {
		var email string
		blRows.Scan(&email)
		blacklisted[strings.ToLower(email)] = true
	}
	blRows.Close()

	// Parse column indexes
	emailIdx := parseColIndex(emailCol)
	nameIdx := parseColIndex(nameCol)

	// Batch size for commits (process 10k at a time)
	const batchSize = 10000
	batchCount := 0

	// Begin first transaction
	tx, err := s.db.Begin()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to begin transaction"})
	}

	// Prepare statement for faster inserts
	stmt, err := tx.Prepare(`INSERT INTO emails (id, list_id, email, name, valid) VALUES ($1, $2, $3, $4, true)`)
	if err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": "Failed to prepare statement"})
	}

	for scanner.Scan() {
		lineNum++

		// Skip header
		if lineNum == 1 && hasHeader {
			continue
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Parse line
		parts := strings.Split(line, delimiter)
		if len(parts) == 0 {
			continue
		}

		// Get email
		var email, name string
		if emailIdx < len(parts) {
			email = strings.TrimSpace(parts[emailIdx])
		}
		if nameIdx >= 0 && nameIdx < len(parts) {
			name = strings.TrimSpace(parts[nameIdx])
		}

		// Validate email
		email = strings.ToLower(email)
		if !emailRegex.MatchString(email) {
			invalidCount++
			continue
		}

		// Check blacklist
		if blacklisted[email] {
			invalidCount++
			continue
		}

		// Check duplicate
		if existingEmails[email] {
			duplicateCount++
			continue
		}

		// Insert email using prepared statement
		emailID := uuid.New().String()
		_, err := stmt.Exec(emailID, id, email, name)

		if err == nil {
			validCount++
			existingEmails[email] = true
			batchCount++

			// Commit every batchSize inserts
			if batchCount >= batchSize {
				stmt.Close()
				tx.Commit()

				// Start new transaction
				tx, err = s.db.Begin()
				if err != nil {
					log.Printf("Failed to begin new transaction: %v", err)
					break
				}
				stmt, err = tx.Prepare(`INSERT INTO emails (id, list_id, email, name, valid) VALUES ($1, $2, $3, $4, true)`)
				if err != nil {
					tx.Rollback()
					log.Printf("Failed to prepare statement: %v", err)
					break
				}
				batchCount = 0
				log.Printf("Processed %d emails so far...", validCount)
			}
		} else {
			invalidCount++
		}
	}

	// Check for scanner errors
	if err := scanner.Err(); err != nil {
		log.Printf("Scanner error during import: %v", err)
		stmt.Close()
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": "Error reading file: " + err.Error()})
	}

	// Commit remaining
	stmt.Close()
	if err := tx.Commit(); err != nil {
		log.Printf("Failed to commit final batch: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to save emails"})
	}

	log.Printf("Import complete: %d valid, %d invalid, %d duplicates out of %d lines", validCount, invalidCount, duplicateCount, lineNum)

	// Update list stats
	s.db.Exec(`
		UPDATE email_lists SET
			total_emails = total_emails + $1,
			valid_emails = valid_emails + $2,
			invalid_emails = invalid_emails + $3,
			status = 'ready'
		WHERE id = $4
	`, validCount+invalidCount, validCount, invalidCount, id)

	return c.JSON(fiber.Map{
		"message":    "Upload complete",
		"valid":      validCount,
		"invalid":    invalidCount,
		"duplicates": duplicateCount,
		"total":      lineNum,
	})
}

// getListEmails returns emails from a list
func (s *Server) getListEmails(c *fiber.Ctx) error {
	id := c.Params("id")
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 50)
	offset := (page - 1) * limit

	// Get total count
	var total int
	s.db.QueryRow(`SELECT COUNT(*) FROM emails WHERE list_id = $1`, id).Scan(&total)

	// Get emails
	rows, err := s.db.Query(`
		SELECT id, email, name, custom1, custom2, custom3, valid, bounced, unsubscribed, created_at
		FROM emails
		WHERE list_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, id, limit, offset)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch emails"})
	}
	defer rows.Close()

	var emails []fiber.Map
	for rows.Next() {
		var emailID, email string
		var name, custom1, custom2, custom3 *string
		var valid, bounced, unsubscribed bool
		var createdAt time.Time

		rows.Scan(&emailID, &email, &name, &custom1, &custom2, &custom3,
			&valid, &bounced, &unsubscribed, &createdAt)

		emails = append(emails, fiber.Map{
			"id":           emailID,
			"email":        email,
			"name":         name,
			"custom1":      custom1,
			"custom2":      custom2,
			"custom3":      custom3,
			"valid":        valid,
			"bounced":      bounced,
			"unsubscribed": unsubscribed,
			"created_at":   createdAt,
		})
	}

	return c.JSON(fiber.Map{
		"data":  emails,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// deleteEmail removes an email from a list
func (s *Server) deleteEmail(c *fiber.Ctx) error {
	listID := c.Params("id")
	emailID := c.Params("emailId")

	result, err := s.db.Exec(`DELETE FROM emails WHERE id = $1 AND list_id = $2`, emailID, listID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete email"})
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "Email not found"})
	}

	// Update list count
	s.db.Exec(`UPDATE email_lists SET total_emails = total_emails - 1, valid_emails = valid_emails - 1 WHERE id = $1`, listID)

	return c.JSON(fiber.Map{"message": "Email deleted"})
}

// parseColIndex parses column index from string
func parseColIndex(s string) int {
	var idx int
	if _, err := strings.NewReader(s).Read([]byte{}); err == nil {
		// It's a number
		for i, c := range s {
			if c >= '0' && c <= '9' {
				idx = idx*10 + int(c-'0')
			} else if i == 0 {
				return -1
			}
		}
	}
	return idx
}
