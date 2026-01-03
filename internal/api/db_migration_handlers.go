package api

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
)

// DatabaseMigrationRequest represents a database migration request
type DatabaseMigrationRequest struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Database string `json:"database"`
	Type     string `json:"type"` // mailwizz, newapp, mumara
}

// DatabaseMigrationTestRequest for testing connection
type DatabaseMigrationTestRequest struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Database string `json:"database"`
	Type     string `json:"type"`
}

// testDatabaseConnection tests connection to external database
func (s *Server) testDatabaseConnection(c *fiber.Ctx) error {
	var req DatabaseMigrationTestRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.Host == "" || req.User == "" || req.Database == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Host, user, and database are required"})
	}

	if req.Port == 0 {
		req.Port = 3306
	}

	// Build MySQL connection string
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True",
		req.User, req.Password, req.Host, req.Port, req.Database)

	// Test connection
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create connection: " + err.Error()})
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to connect: " + err.Error()})
	}

	// Get table name and count based on type
	var tableName, emailColumn string
	switch req.Type {
	case "mailwizz":
		tableName = "mw_email_blacklist"
		emailColumn = "email"
	case "newapp":
		tableName = "email_banned_emails"
		emailColumn = "emailaddress"
	case "mumara":
		tableName = "suppression_list"
		emailColumn = "email"
	default:
		return c.Status(400).JSON(fiber.Map{"error": "Invalid database type. Must be: mailwizz, newapp, or mumara"})
	}

	// Count emails in source table
	var count int
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName)
	err = db.QueryRow(query).Scan(&count)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": fmt.Sprintf("Failed to query table %s: %s", tableName, err.Error()),
		})
	}

	// Get sample emails
	var samples []string
	sampleQuery := fmt.Sprintf("SELECT %s FROM %s LIMIT 5", emailColumn, tableName)
	rows, err := db.Query(sampleQuery)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var email string
			if rows.Scan(&email) == nil {
				samples = append(samples, email)
			}
		}
	}

	return c.JSON(fiber.Map{
		"message":      "Connection successful",
		"table":        tableName,
		"email_column": emailColumn,
		"total_emails": count,
		"samples":      samples,
	})
}

// migrateFromDatabase migrates blacklist from external database
func (s *Server) migrateFromDatabase(c *fiber.Ctx) error {
	var req DatabaseMigrationRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.Host == "" || req.User == "" || req.Database == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Host, user, and database are required"})
	}

	if req.Port == 0 {
		req.Port = 3306
	}

	// Build MySQL connection string
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True",
		req.User, req.Password, req.Host, req.Port, req.Database)

	// Connect to external database
	extDB, err := sql.Open("mysql", dsn)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to connect to external database: " + err.Error()})
	}
	defer extDB.Close()

	if err := extDB.Ping(); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to ping external database: " + err.Error()})
	}

	// Get table name and email column based on type
	var tableName, emailColumn, reasonColumn string
	switch req.Type {
	case "mailwizz":
		tableName = "mw_email_blacklist"
		emailColumn = "email"
		reasonColumn = "reason"
	case "newapp":
		tableName = "email_banned_emails"
		emailColumn = "emailaddress"
		reasonColumn = "" // No reason column in newapp
	case "mumara":
		tableName = "suppression_list"
		emailColumn = "email"
		reasonColumn = "type"
	default:
		return c.Status(400).JSON(fiber.Map{"error": "Invalid database type. Must be: mailwizz, newapp, or mumara"})
	}

	// Query emails from external database
	var query string
	if reasonColumn != "" {
		query = fmt.Sprintf("SELECT %s, %s FROM %s", emailColumn, reasonColumn, tableName)
	} else {
		query = fmt.Sprintf("SELECT %s FROM %s", emailColumn, tableName)
	}

	rows, err := extDB.Query(query)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to query external database: " + err.Error()})
	}
	defer rows.Close()

	// Begin transaction in local database
	tx, err := s.db.Begin()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to begin transaction"})
	}

	imported := 0
	duplicates := 0
	errors := 0

	// Process emails
	for rows.Next() {
		var email string
		var reason string

		if reasonColumn != "" {
			var reasonPtr *string
			if err := rows.Scan(&email, &reasonPtr); err != nil {
				errors++
				continue
			}
			if reasonPtr != nil {
				reason = *reasonPtr
			}
		} else {
			if err := rows.Scan(&email); err != nil {
				errors++
				continue
			}
		}

		// Clean and validate email
		email = strings.ToLower(strings.TrimSpace(email))
		if email == "" || !emailRegex.MatchString(email) {
			errors++
			continue
		}

		// Set default reason based on source
		if reason == "" {
			reason = fmt.Sprintf("import_%s", req.Type)
		} else {
			// Normalize reason from source
			reason = normalizeReason(reason, req.Type)
		}

		// Insert into blacklist
		id := uuid.New().String()
		result, err := tx.Exec(`
			INSERT INTO blacklist (id, email, reason)
			VALUES ($1, $2, $3)
			ON CONFLICT (email) DO NOTHING
		`, id, email, reason)

		if err != nil {
			errors++
			continue
		}

		rowsAffected, _ := result.RowsAffected()
		if rowsAffected > 0 {
			imported++
		} else {
			duplicates++
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to commit transaction"})
	}

	// Update emails table to mark blacklisted emails as invalid
	s.db.Exec(`
		UPDATE emails SET valid = false
		WHERE LOWER(email) IN (SELECT email FROM blacklist)
	`)

	return c.JSON(fiber.Map{
		"message":    "Migration completed",
		"source":     req.Type,
		"imported":   imported,
		"duplicates": duplicates,
		"errors":     errors,
		"total":      imported + duplicates + errors,
	})
}

// normalizeReason normalizes reason strings from different sources
func normalizeReason(reason string, sourceType string) string {
	reason = strings.ToLower(strings.TrimSpace(reason))

	// Normalize common reason values
	switch {
	case strings.Contains(reason, "bounce"):
		return "bounce"
	case strings.Contains(reason, "unsubscribe"):
		return "unsubscribe"
	case strings.Contains(reason, "complaint") || strings.Contains(reason, "spam"):
		return "complaint"
	case strings.Contains(reason, "manual"):
		return "manual"
	case strings.Contains(reason, "spamtrap"):
		return "spamtrap"
	case strings.Contains(reason, "invalid"):
		return "invalid"
	default:
		return fmt.Sprintf("import_%s", sourceType)
	}
}

// getMigrationStats returns stats about potential migration sources
func (s *Server) getMigrationStats(c *fiber.Ctx) error {
	// Return information about supported database types
	return c.JSON(fiber.Map{
		"supported_databases": []fiber.Map{
			{
				"type":         "mailwizz",
				"name":         "MailWizz",
				"table":        "mw_email_blacklist",
				"email_column": "email",
				"description":  "MailWizz email marketing platform blacklist",
			},
			{
				"type":         "newapp",
				"name":         "NewApp",
				"table":        "email_banned_emails",
				"email_column": "emailaddress",
				"description":  "NewApp banned emails list",
			},
			{
				"type":         "mumara",
				"name":         "Mumara",
				"table":        "suppression_list",
				"email_column": "email",
				"description":  "Mumara suppression list",
			},
		},
	})
}
