package api

import (
	"bufio"
	"regexp"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// SQLMigrationRequest represents the SQL file migration request
type SQLMigrationRequest struct {
	Type string `json:"type"` // mailwizz, newapp, mumara
}

// importFromSQLFile imports blacklist from SQL dump file
func (s *Server) importFromSQLFile(c *fiber.Ctx) error {
	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Nenhum arquivo enviado"})
	}

	dbType := c.FormValue("type", "mailwizz")

	// Validate database type
	var tableName, emailColumn string
	var emailIndex int
	switch dbType {
	case "mailwizz":
		tableName = "mw_email_blacklist"
		emailColumn = "email"
		emailIndex = 2 // email_id, subscriber_id, email, reason, date_added, last_updated
	case "newapp":
		tableName = "email_banned_emails"
		emailColumn = "emailaddress"
		emailIndex = 1 // banid, emailaddress, list, bandate
	case "mumara":
		tableName = "suppression_list"
		emailColumn = "email"
		emailIndex = 1 // suppression_list_id, email, account_id_fk, type, list_id_fk, create_time
	default:
		return c.Status(400).JSON(fiber.Map{"error": "Tipo de banco invalido. Use: mailwizz, newapp, ou mumara"})
	}

	f, err := file.Open()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Erro ao abrir arquivo"})
	}
	defer f.Close()

	// Parse SQL file and extract emails
	emails := make(map[string]bool) // Use map to deduplicate
	scanner := bufio.NewScanner(f)

	// Increase buffer size for large SQL files with long lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 10*1024*1024) // 10MB max line size

	// Regex to match INSERT statements
	insertRegex := regexp.MustCompile(`(?i)INSERT\s+INTO\s+` + "`?" + tableName + "`?" + `\s+`)

	// Regex to extract values from INSERT
	valuesRegex := regexp.MustCompile(`\(([^)]+)\)`)

	for scanner.Scan() {
		line := scanner.Text()

		// Check if line contains INSERT for our table
		if !insertRegex.MatchString(line) {
			continue
		}

		// Find all value groups in the line
		matches := valuesRegex.FindAllStringSubmatch(line, -1)
		for _, match := range matches {
			if len(match) < 2 {
				continue
			}

			// Parse the values
			values := parseInsertValues(match[1])
			if emailIndex < len(values) {
				email := cleanEmail(values[emailIndex])
				if email != "" && emailRegex.MatchString(email) {
					emails[email] = true
				}
			}
		}
	}

	if len(emails) == 0 {
		return c.Status(400).JSON(fiber.Map{
			"error": "Nenhum email encontrado no arquivo. Verifique se o arquivo contem a tabela " + tableName,
		})
	}

	// Begin transaction
	tx, err := s.db.Begin()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Erro ao iniciar transacao"})
	}

	imported := 0
	duplicates := 0
	reason := "import_" + dbType

	for email := range emails {
		id := uuid.New().String()
		result, err := tx.Exec(`
			INSERT INTO blacklist (id, email, reason)
			VALUES ($1, $2, $3)
			ON CONFLICT (email) DO NOTHING
		`, id, email, reason)

		if err != nil {
			continue
		}

		rows, _ := result.RowsAffected()
		if rows > 0 {
			imported++
		} else {
			duplicates++
		}
	}

	if err := tx.Commit(); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Erro ao salvar dados"})
	}

	// Update emails table
	s.db.Exec(`
		UPDATE emails SET valid = false
		WHERE LOWER(email) IN (SELECT email FROM blacklist)
	`)

	return c.JSON(fiber.Map{
		"message":      "Importacao concluida",
		"source":       dbType,
		"table":        tableName,
		"email_column": emailColumn,
		"imported":     imported,
		"duplicates":   duplicates,
		"total":        imported + duplicates,
	})
}

// parseInsertValues parses comma-separated values from SQL INSERT
func parseInsertValues(valuesStr string) []string {
	var values []string
	var current strings.Builder
	inQuote := false
	quoteChar := rune(0)
	escaped := false

	for _, char := range valuesStr {
		if escaped {
			current.WriteRune(char)
			escaped = false
			continue
		}

		if char == '\\' {
			escaped = true
			continue
		}

		if (char == '\'' || char == '"') && !inQuote {
			inQuote = true
			quoteChar = char
			continue
		}

		if char == quoteChar && inQuote {
			inQuote = false
			quoteChar = 0
			continue
		}

		if char == ',' && !inQuote {
			values = append(values, strings.TrimSpace(current.String()))
			current.Reset()
			continue
		}

		current.WriteRune(char)
	}

	// Add last value
	if current.Len() > 0 {
		values = append(values, strings.TrimSpace(current.String()))
	}

	return values
}

// cleanEmail removes quotes and trims whitespace from email
func cleanEmail(email string) string {
	email = strings.TrimSpace(email)
	email = strings.Trim(email, "'\"")
	email = strings.ToLower(email)
	return email
}

// previewSQLFile previews the SQL file without importing
func (s *Server) previewSQLFile(c *fiber.Ctx) error {
	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Nenhum arquivo enviado"})
	}

	dbType := c.FormValue("type", "mailwizz")

	// Validate database type
	var tableName string
	var emailIndex int
	switch dbType {
	case "mailwizz":
		tableName = "mw_email_blacklist"
		emailIndex = 2
	case "newapp":
		tableName = "email_banned_emails"
		emailIndex = 1
	case "mumara":
		tableName = "suppression_list"
		emailIndex = 1
	default:
		return c.Status(400).JSON(fiber.Map{"error": "Tipo de banco invalido"})
	}

	f, err := file.Open()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Erro ao abrir arquivo"})
	}
	defer f.Close()

	emails := make(map[string]bool)
	var samples []string
	scanner := bufio.NewScanner(f)

	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 10*1024*1024)

	insertRegex := regexp.MustCompile(`(?i)INSERT\s+INTO\s+` + "`?" + tableName + "`?" + `\s+`)
	valuesRegex := regexp.MustCompile(`\(([^)]+)\)`)

	for scanner.Scan() {
		line := scanner.Text()

		if !insertRegex.MatchString(line) {
			continue
		}

		matches := valuesRegex.FindAllStringSubmatch(line, -1)
		for _, match := range matches {
			if len(match) < 2 {
				continue
			}

			values := parseInsertValues(match[1])
			if emailIndex < len(values) {
				email := cleanEmail(values[emailIndex])
				if email != "" && emailRegex.MatchString(email) {
					if !emails[email] {
						emails[email] = true
						if len(samples) < 10 {
							samples = append(samples, email)
						}
					}
				}
			}
		}
	}

	if len(emails) == 0 {
		return c.Status(400).JSON(fiber.Map{
			"error": "Nenhum email encontrado. Verifique se o arquivo contem a tabela " + tableName,
		})
	}

	return c.JSON(fiber.Map{
		"message":      "Arquivo analisado",
		"table":        tableName,
		"total_emails": len(emails),
		"samples":      samples,
	})
}

// getMigrationStats returns info about supported database types
func (s *Server) getMigrationStats(c *fiber.Ctx) error {
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
