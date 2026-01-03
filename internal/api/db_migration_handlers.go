package api

import (
	"bufio"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// importFromSQLFile imports blacklist from SQL dump file
func (s *Server) importFromSQLFile(c *fiber.Ctx) error {
	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Nenhum arquivo enviado"})
	}

	dbType := c.FormValue("type", "mailwizz")

	// Validate database type and get email column index
	var tableName string
	var emailIndex int
	switch dbType {
	case "mailwizz":
		tableName = "mw_email_blacklist"
		emailIndex = 2 // (email_id, subscriber_id, EMAIL, reason, date_added, last_updated)
	case "newapp":
		tableName = "email_banned_emails"
		emailIndex = 1 // (banid, EMAILADDRESS, list, bandate)
	case "mumara":
		tableName = "suppression_list"
		emailIndex = 1 // (suppression_list_id, EMAIL, account_id_fk, type, list_id_fk, create_time)
	default:
		return c.Status(400).JSON(fiber.Map{"error": "Tipo de banco invalido. Use: mailwizz, newapp, ou mumara"})
	}

	f, err := file.Open()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Erro ao abrir arquivo"})
	}
	defer f.Close()

	// Parse SQL file and extract emails
	emails := make(map[string]bool)
	scanner := bufio.NewScanner(f)

	// Increase buffer size for large SQL files
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 50*1024*1024) // 50MB max line size

	tableNameLower := strings.ToLower(tableName)

	for scanner.Scan() {
		line := scanner.Text()
		lineLower := strings.ToLower(line)

		// Check if line contains INSERT INTO our table
		if !strings.Contains(lineLower, "insert into") {
			continue
		}
		if !strings.Contains(lineLower, tableNameLower) {
			continue
		}

		// Extract all values tuples from the line
		extractedEmails := extractEmailsFromLine(line, emailIndex)
		for _, email := range extractedEmails {
			if email != "" && emailRegex.MatchString(email) {
				emails[email] = true
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
		"message":    "Importacao concluida",
		"source":     dbType,
		"table":      tableName,
		"imported":   imported,
		"duplicates": duplicates,
		"total":      imported + duplicates,
	})
}

// extractEmailsFromLine extracts emails from a SQL INSERT line
func extractEmailsFromLine(line string, emailIndex int) []string {
	var emails []string

	// Find VALUES keyword
	valuesIdx := strings.Index(strings.ToLower(line), "values")
	if valuesIdx == -1 {
		return emails
	}

	// Get everything after VALUES
	valuesStr := line[valuesIdx+6:]

	// Parse each tuple (value1, value2, ...)
	tuples := parseTuples(valuesStr)

	for _, tuple := range tuples {
		values := parseTupleValues(tuple)
		if emailIndex < len(values) {
			email := cleanEmailValue(values[emailIndex])
			if email != "" {
				emails = append(emails, email)
			}
		}
	}

	return emails
}

// parseTuples splits "(v1,v2),(v3,v4)" into ["v1,v2", "v3,v4"]
func parseTuples(s string) []string {
	var tuples []string
	var current strings.Builder
	depth := 0
	inQuote := false
	quoteChar := rune(0)

	for _, char := range s {
		// Handle escape sequences
		if inQuote && char == '\\' {
			current.WriteRune(char)
			continue
		}

		// Handle quotes
		if (char == '\'' || char == '"') && !inQuote {
			inQuote = true
			quoteChar = char
			current.WriteRune(char)
			continue
		}
		if char == quoteChar && inQuote {
			inQuote = false
			quoteChar = 0
			current.WriteRune(char)
			continue
		}

		if !inQuote {
			if char == '(' {
				if depth == 0 {
					current.Reset()
				} else {
					current.WriteRune(char)
				}
				depth++
				continue
			}
			if char == ')' {
				depth--
				if depth == 0 {
					tuples = append(tuples, current.String())
					current.Reset()
				} else {
					current.WriteRune(char)
				}
				continue
			}
		}

		if depth > 0 {
			current.WriteRune(char)
		}
	}

	return tuples
}

// parseTupleValues splits "v1, v2, v3" into ["v1", "v2", "v3"]
func parseTupleValues(tuple string) []string {
	var values []string
	var current strings.Builder
	inQuote := false
	quoteChar := rune(0)
	escaped := false

	for _, char := range tuple {
		if escaped {
			current.WriteRune(char)
			escaped = false
			continue
		}

		if char == '\\' && inQuote {
			escaped = true
			current.WriteRune(char)
			continue
		}

		if (char == '\'' || char == '"') && !inQuote {
			inQuote = true
			quoteChar = char
			current.WriteRune(char)
			continue
		}

		if char == quoteChar && inQuote {
			inQuote = false
			quoteChar = 0
			current.WriteRune(char)
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
	if current.Len() > 0 || len(values) > 0 {
		values = append(values, strings.TrimSpace(current.String()))
	}

	return values
}

// cleanEmailValue removes quotes and converts to lowercase
func cleanEmailValue(val string) string {
	val = strings.TrimSpace(val)
	// Remove surrounding quotes
	if len(val) >= 2 {
		if (val[0] == '\'' && val[len(val)-1] == '\'') ||
			(val[0] == '"' && val[len(val)-1] == '"') {
			val = val[1 : len(val)-1]
		}
	}
	return strings.ToLower(val)
}

// previewSQLFile previews the SQL file without importing
func (s *Server) previewSQLFile(c *fiber.Ctx) error {
	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Nenhum arquivo enviado"})
	}

	dbType := c.FormValue("type", "mailwizz")

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
	scanner.Buffer(buf, 50*1024*1024)

	tableNameLower := strings.ToLower(tableName)

	for scanner.Scan() {
		line := scanner.Text()
		lineLower := strings.ToLower(line)

		if !strings.Contains(lineLower, "insert into") {
			continue
		}
		if !strings.Contains(lineLower, tableNameLower) {
			continue
		}

		extractedEmails := extractEmailsFromLine(line, emailIndex)
		for _, email := range extractedEmails {
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
				"type":        "mailwizz",
				"name":        "MailWizz",
				"table":       "mw_email_blacklist",
				"description": "MailWizz blacklist",
			},
			{
				"type":        "newapp",
				"name":        "NewApp",
				"table":       "email_banned_emails",
				"description": "NewApp banned emails",
			},
			{
				"type":        "mumara",
				"name":        "Mumara",
				"table":       "suppression_list",
				"description": "Mumara suppression list",
			},
		},
	})
}
