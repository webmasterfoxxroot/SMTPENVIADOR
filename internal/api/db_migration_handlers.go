package api

import (
	"bufio"
	"io"
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

	// Read entire file content
	content, err := io.ReadAll(f)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Erro ao ler arquivo"})
	}

	// Extract emails from SQL content
	emails := extractEmailsFromSQL(string(content), tableName, emailIndex)

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

// extractEmailsFromSQL extracts emails from SQL content
func extractEmailsFromSQL(content string, tableName string, emailIndex int) map[string]bool {
	emails := make(map[string]bool)
	tableNameLower := strings.ToLower(tableName)
	contentLower := strings.ToLower(content)

	// Check if table exists in content
	if !strings.Contains(contentLower, tableNameLower) {
		return emails
	}

	// Find all value tuples in the content
	// Pattern: (value1, value2, value3, ...)
	tuples := extractAllTuples(content)

	for _, tuple := range tuples {
		values := parseTupleValues(tuple)
		if emailIndex < len(values) {
			email := cleanEmailValue(values[emailIndex])
			if email != "" && emailRegex.MatchString(email) {
				emails[email] = true
			}
		}
	}

	return emails
}

// extractAllTuples extracts all (value, value, ...) tuples from SQL content
func extractAllTuples(content string) []string {
	var tuples []string
	var current strings.Builder
	depth := 0
	inQuote := false
	quoteChar := rune(0)
	prevChar := rune(0)

	for _, char := range content {
		// Handle escape sequences
		if prevChar == '\\' && inQuote {
			current.WriteRune(char)
			prevChar = char
			continue
		}

		// Handle quotes
		if (char == '\'' || char == '"') && prevChar != '\\' {
			if !inQuote {
				inQuote = true
				quoteChar = char
			} else if char == quoteChar {
				inQuote = false
				quoteChar = 0
			}
		}

		if !inQuote {
			if char == '(' {
				if depth == 0 {
					current.Reset()
				} else {
					current.WriteRune(char)
				}
				depth++
				prevChar = char
				continue
			}
			if char == ')' {
				depth--
				if depth == 0 {
					tuple := current.String()
					// Only add if it looks like data (contains comma and quote)
					if strings.Contains(tuple, ",") && (strings.Contains(tuple, "'") || strings.Contains(tuple, "\"")) {
						tuples = append(tuples, tuple)
					}
					current.Reset()
				} else {
					current.WriteRune(char)
				}
				prevChar = char
				continue
			}
		}

		if depth > 0 {
			current.WriteRune(char)
		}

		prevChar = char
	}

	return tuples
}

// parseTupleValues splits "v1, v2, v3" into ["v1", "v2", "v3"]
func parseTupleValues(tuple string) []string {
	var values []string
	var current strings.Builder
	inQuote := false
	quoteChar := rune(0)
	prevChar := rune(0)

	for _, char := range tuple {
		// Handle escape
		if prevChar == '\\' && inQuote {
			current.WriteRune(char)
			prevChar = char
			continue
		}

		// Handle quotes
		if (char == '\'' || char == '"') && prevChar != '\\' {
			if !inQuote {
				inQuote = true
				quoteChar = char
				current.WriteRune(char)
			} else if char == quoteChar {
				inQuote = false
				quoteChar = 0
				current.WriteRune(char)
			} else {
				current.WriteRune(char)
			}
			prevChar = char
			continue
		}

		if char == ',' && !inQuote {
			values = append(values, strings.TrimSpace(current.String()))
			current.Reset()
			prevChar = char
			continue
		}

		current.WriteRune(char)
		prevChar = char
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
	// Handle NULL values
	if strings.ToUpper(val) == "NULL" {
		return ""
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

	// For preview, read file in chunks to handle large files
	// but limit to first 50MB for preview
	content, err := io.ReadAll(io.LimitReader(f, 50*1024*1024))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Erro ao ler arquivo"})
	}

	emails := extractEmailsFromSQL(string(content), tableName, emailIndex)

	if len(emails) == 0 {
		return c.Status(400).JSON(fiber.Map{
			"error": "Nenhum email encontrado. Verifique se o arquivo contem a tabela " + tableName,
		})
	}

	// Get samples
	var samples []string
	count := 0
	for email := range emails {
		if count < 10 {
			samples = append(samples, email)
			count++
		} else {
			break
		}
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

// Legacy function for line-by-line parsing (kept for reference)
func extractEmailsFromLine(line string, emailIndex int) []string {
	var emails []string
	scanner := bufio.NewScanner(strings.NewReader(line))
	for scanner.Scan() {
		tuples := extractAllTuples(scanner.Text())
		for _, tuple := range tuples {
			values := parseTupleValues(tuple)
			if emailIndex < len(values) {
				email := cleanEmailValue(values[emailIndex])
				if email != "" {
					emails = append(emails, email)
				}
			}
		}
	}
	return emails
}
