package api

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// startBlacklistImport starts an async import job for blacklist SQL file
func (s *Server) startBlacklistImport(c *fiber.Ctx) error {
	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Nenhum arquivo enviado"})
	}

	dbType := c.FormValue("type", "mailwizz")

	// Validate database type
	var tableName string
	switch dbType {
	case "mailwizz":
		tableName = "mw_email_blacklist"
	case "newapp":
		tableName = "email_banned_emails"
	case "mumara":
		tableName = "suppression_list"
	default:
		return c.Status(400).JSON(fiber.Map{"error": "Tipo de banco invalido. Use: mailwizz, newapp, ou mumara"})
	}

	// Create uploads directory
	uploadDir := "/tmp/smtpenviador/blacklist_uploads"
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Falha ao criar diretorio"})
	}

	// Generate job ID and save file
	jobID := uuid.New().String()
	ext := filepath.Ext(file.Filename)
	if ext == "" {
		ext = ".sql"
	}
	savedPath := filepath.Join(uploadDir, fmt.Sprintf("%s%s", jobID, ext))

	// Save file to disk
	src, err := file.Open()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Falha ao abrir arquivo"})
	}
	defer src.Close()

	dst, err := os.Create(savedPath)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Falha ao salvar arquivo"})
	}

	written, err := io.Copy(dst, src)
	dst.Close()
	if err != nil {
		os.Remove(savedPath)
		return c.Status(500).JSON(fiber.Map{"error": "Falha ao copiar arquivo"})
	}

	log.Printf("[BlacklistImport] File saved: %s (%d bytes)", savedPath, written)

	// Create job in database
	_, err = s.db.Exec(`
		INSERT INTO blacklist_import_jobs (id, file_name, file_path, db_type, status, started_at)
		VALUES ($1, $2, $3, $4, 'pending', NOW())
	`, jobID, file.Filename, savedPath, dbType)
	if err != nil {
		os.Remove(savedPath)
		log.Printf("[BlacklistImport] Failed to create job: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Falha ao criar job de importacao"})
	}

	log.Printf("[BlacklistImport] Job created: ID=%s, Type=%s, Table=%s", jobID, dbType, tableName)

	// Start background processing
	go s.processBlacklistImportJob(jobID)

	return c.JSON(fiber.Map{
		"message":   "Importacao iniciada",
		"job_id":    jobID,
		"file_name": file.Filename,
		"file_size": written,
		"db_type":   dbType,
		"table":     tableName,
	})
}

// getBlacklistImportStatus returns the status of a blacklist import job
func (s *Server) getBlacklistImportStatus(c *fiber.Ctx) error {
	jobID := c.Params("jobId")

	var status, dbType, fileName string
	var totalEmails, processed, imported, duplicates, errors int
	var errorMessage *string
	var startedAt, completedAt *time.Time

	err := s.db.QueryRow(`
		SELECT status, db_type, file_name, total_emails, processed, imported, duplicates, errors, error_message, started_at, completed_at
		FROM blacklist_import_jobs
		WHERE id = $1
	`, jobID).Scan(&status, &dbType, &fileName, &totalEmails, &processed, &imported, &duplicates, &errors, &errorMessage, &startedAt, &completedAt)

	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Job nao encontrado"})
	}

	// Calculate progress percentage
	progress := 0
	if totalEmails > 0 {
		progress = (processed * 100) / totalEmails
	}

	return c.JSON(fiber.Map{
		"job_id":        jobID,
		"status":        status,
		"db_type":       dbType,
		"file_name":     fileName,
		"total_emails":  totalEmails,
		"processed":     processed,
		"imported":      imported,
		"duplicates":    duplicates,
		"errors":        errors,
		"error_message": errorMessage,
		"progress":      progress,
		"started_at":    startedAt,
		"completed_at":  completedAt,
	})
}

// processBlacklistImportJob processes the SQL file in background
func (s *Server) processBlacklistImportJob(jobID string) {
	log.Printf("[BlacklistImport] Starting job: %s", jobID)

	// Helper functions
	updateStatus := func(status string, errorMsg string) {
		if errorMsg != "" {
			s.db.Exec(`UPDATE blacklist_import_jobs SET status = $1, error_message = $2, updated_at = NOW() WHERE id = $3`,
				status, errorMsg, jobID)
		} else {
			s.db.Exec(`UPDATE blacklist_import_jobs SET status = $1, updated_at = NOW() WHERE id = $2`, status, jobID)
		}
	}

	updateProgress := func(total, processed, imported, duplicates, errors int) {
		s.db.Exec(`UPDATE blacklist_import_jobs SET total_emails = $1, processed = $2, imported = $3, duplicates = $4, errors = $5, updated_at = NOW() WHERE id = $6`,
			total, processed, imported, duplicates, errors, jobID)
	}

	// Recover from panics
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[BlacklistImport] Job %s PANIC: %v", jobID, r)
			updateStatus("failed", fmt.Sprintf("Erro interno: %v", r))
		}
	}()

	// Read job details
	var filePath, dbType string
	err := s.db.QueryRow(`SELECT file_path, db_type FROM blacklist_import_jobs WHERE id = $1`, jobID).Scan(&filePath, &dbType)
	if err != nil {
		log.Printf("[BlacklistImport] Job %s: failed to read job: %v", jobID, err)
		updateStatus("failed", "Job nao encontrado")
		return
	}

	// Update status to processing
	updateStatus("processing", "")

	// Cleanup file after processing
	defer func() {
		os.Remove(filePath)
		log.Printf("[BlacklistImport] Job %s: cleaned up file", jobID)
	}()

	// Read file
	content, err := os.ReadFile(filePath)
	if err != nil {
		updateStatus("failed", "Falha ao ler arquivo: "+err.Error())
		return
	}

	// Get email index based on db type
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
	}

	// Check if table exists in content
	if !strings.Contains(strings.ToLower(string(content)), strings.ToLower(tableName)) {
		updateStatus("failed", "Tabela "+tableName+" nao encontrada no arquivo")
		return
	}

	// Extract all tuples
	tuples := extractAllTuples(string(content))
	totalEmails := len(tuples)

	if totalEmails == 0 {
		updateStatus("failed", "Nenhum email encontrado no arquivo")
		return
	}

	log.Printf("[BlacklistImport] Job %s: found %d tuples to process", jobID, totalEmails)
	updateProgress(totalEmails, 0, 0, 0, 0)

	// Process in batches
	imported := 0
	duplicates := 0
	errors := 0
	processed := 0
	batchSize := 1000
	reason := "import_" + dbType

	// Begin transaction for batch
	tx, err := s.db.Begin()
	if err != nil {
		updateStatus("failed", "Falha ao iniciar transacao")
		return
	}

	for i, tuple := range tuples {
		values := parseTupleValues(tuple)
		if emailIndex < len(values) {
			email := cleanEmailValue(values[emailIndex])
			if email != "" && emailRegex.MatchString(email) {
				id := uuid.New().String()
				result, err := tx.Exec(`
					INSERT INTO blacklist (id, email, reason)
					VALUES ($1, $2, $3)
					ON CONFLICT (email) DO NOTHING
				`, id, email, reason)

				if err != nil {
					errors++
				} else {
					rows, _ := result.RowsAffected()
					if rows > 0 {
						imported++
					} else {
						duplicates++
					}
				}
			} else {
				errors++
			}
		} else {
			errors++
		}

		processed++

		// Update progress every batch
		if processed%batchSize == 0 || processed == totalEmails {
			updateProgress(totalEmails, processed, imported, duplicates, errors)
			log.Printf("[BlacklistImport] Job %s: progress %d/%d (%d%%)", jobID, processed, totalEmails, (processed*100)/totalEmails)
		}

		// Commit and start new transaction every 10000 records
		if (i+1)%10000 == 0 && i < totalEmails-1 {
			tx.Commit()
			tx, _ = s.db.Begin()
		}
	}

	// Final commit
	if err := tx.Commit(); err != nil {
		updateStatus("failed", "Falha ao salvar dados")
		return
	}

	// Update emails table
	s.db.Exec(`
		UPDATE emails SET valid = false
		WHERE LOWER(email) IN (SELECT email FROM blacklist)
	`)

	// Mark as completed
	s.db.Exec(`UPDATE blacklist_import_jobs SET status = 'completed', completed_at = NOW(), updated_at = NOW() WHERE id = $1`, jobID)

	log.Printf("[BlacklistImport] Job %s: COMPLETED - imported=%d, duplicates=%d, errors=%d", jobID, imported, duplicates, errors)
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

// listBlacklistImportJobs returns all blacklist import jobs
func (s *Server) listBlacklistImportJobs(c *fiber.Ctx) error {
	rows, err := s.db.Query(`
		SELECT id, file_name, db_type, status, total_emails, processed, imported, duplicates, errors, started_at, completed_at
		FROM blacklist_import_jobs
		ORDER BY created_at DESC
		LIMIT 20
	`)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Falha ao buscar jobs"})
	}
	defer rows.Close()

	var jobs []fiber.Map
	for rows.Next() {
		var id, fileName, dbType, status string
		var totalEmails, processed, imported, duplicates, errors int
		var startedAt, completedAt *time.Time

		rows.Scan(&id, &fileName, &dbType, &status, &totalEmails, &processed, &imported, &duplicates, &errors, &startedAt, &completedAt)

		progress := 0
		if totalEmails > 0 {
			progress = (processed * 100) / totalEmails
		}

		jobs = append(jobs, fiber.Map{
			"id":           id,
			"file_name":    fileName,
			"db_type":      dbType,
			"status":       status,
			"total_emails": totalEmails,
			"processed":    processed,
			"imported":     imported,
			"duplicates":   duplicates,
			"errors":       errors,
			"progress":     progress,
			"started_at":   startedAt,
			"completed_at": completedAt,
		})
	}

	return c.JSON(fiber.Map{"data": jobs})
}
