package api

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type EmailListRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ImportJob tracks the status of a background import
type ImportJob struct {
	ID           string    `json:"id"`
	ListID       string    `json:"list_id"`
	FileName     string    `json:"file_name"`
	Status       string    `json:"status"` // pending, processing, completed, failed
	TotalLines   int       `json:"total_lines"`
	Processed    int       `json:"processed"`
	Valid        int       `json:"valid"`
	Invalid      int       `json:"invalid"`
	Duplicates   int       `json:"duplicates"`
	Error        string    `json:"error,omitempty"`
	StartedAt    time.Time `json:"started_at"`
	CompletedAt  time.Time `json:"completed_at,omitempty"`
}

// Import job storage (in-memory, could be moved to Redis/DB for persistence)
var (
	importJobs   = make(map[string]*ImportJob)
	importJobsMu sync.RWMutex
)

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

// deleteEmailList deletes an email list and all its emails
func (s *Server) deleteEmailList(c *fiber.Ctx) error {
	id := c.Params("id")

	// Start transaction
	tx, err := s.db.Begin()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to start transaction"})
	}

	// First delete all emails in this list
	_, err = tx.Exec(`DELETE FROM emails WHERE list_id = $1`, id)
	if err != nil {
		tx.Rollback()
		log.Printf("Failed to delete emails for list %s: %v", id, err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete emails"})
	}

	// Then delete the list
	result, err := tx.Exec(`DELETE FROM email_lists WHERE id = $1`, id)
	if err != nil {
		tx.Rollback()
		log.Printf("Failed to delete list %s: %v", id, err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete email list"})
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		tx.Rollback()
		return c.Status(404).JSON(fiber.Map{"error": "Email list not found"})
	}

	if err := tx.Commit(); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to commit deletion"})
	}

	log.Printf("Deleted email list %s with all its emails", id)
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
	stmt, err := tx.Prepare(`INSERT INTO emails (id, list_id, email, name, valid) VALUES ($1, $2, $3, $4, true) ON CONFLICT (list_id, email) DO NOTHING`)
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
				stmt, err = tx.Prepare(`INSERT INTO emails (id, list_id, email, name, valid) VALUES ($1, $2, $3, $4, true) ON CONFLICT (list_id, email) DO NOTHING`)
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

// uploadEmailsAsync handles large file uploads asynchronously
// The file is saved to disk first, then processed in the background
func (s *Server) uploadEmailsAsync(c *fiber.Ctx) error {
	listID := c.Params("id")

	// Check if list exists
	var exists bool
	s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM email_lists WHERE id = $1)`, listID).Scan(&exists)
	if !exists {
		return c.Status(404).JSON(fiber.Map{"error": "Lista nao encontrada"})
	}

	// Get file
	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Nenhum arquivo enviado"})
	}

	// Check file size against configured limit
	maxSizeMB := s.getMaxUploadSizeMB()
	maxSizeBytes := maxSizeMB * 1024 * 1024
	if file.Size > maxSizeBytes {
		return c.Status(413).JSON(fiber.Map{
			"error": fmt.Sprintf("Arquivo muito grande. Maximo permitido: %d MB. Seu arquivo: %d MB", maxSizeMB, file.Size/(1024*1024)),
		})
	}

	// Get options
	hasHeader := c.FormValue("has_header", "true") == "true"
	delimiter := c.FormValue("delimiter", ",")

	// Create uploads directory if not exists
	uploadDir := "/tmp/smtpenviador/uploads"
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Falha ao criar diretorio de uploads"})
	}

	// Generate unique filename
	jobID := uuid.New().String()
	ext := filepath.Ext(file.Filename)
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
	defer dst.Close()

	// Copy file to disk
	written, err := io.Copy(dst, src)
	if err != nil {
		os.Remove(savedPath)
		return c.Status(500).JSON(fiber.Map{"error": "Falha ao copiar arquivo"})
	}

	log.Printf("File saved: %s (%d bytes)", savedPath, written)

	// Count lines in file for progress tracking
	dst.Seek(0, 0)
	lineCount := 0
	scanner := bufio.NewScanner(dst)
	for scanner.Scan() {
		lineCount++
	}
	if hasHeader && lineCount > 0 {
		lineCount--
	}

	// Create import job
	job := &ImportJob{
		ID:         jobID,
		ListID:     listID,
		FileName:   file.Filename,
		Status:     "pending",
		TotalLines: lineCount,
		StartedAt:  time.Now(),
	}

	log.Printf("Creating import job: ID=%s, ListID=%s, FileName=%s, TotalLines=%d", jobID, listID, file.Filename, lineCount)

	importJobsMu.Lock()
	importJobs[jobID] = job
	importJobsMu.Unlock()

	// Update list status
	s.db.Exec(`UPDATE email_lists SET status = 'importing' WHERE id = $1`, listID)

	// Start background processing
	go s.processImportJob(jobID, savedPath, listID, hasHeader, delimiter)

	return c.JSON(fiber.Map{
		"message":     "Upload iniciado",
		"job_id":      jobID,
		"total_lines": lineCount,
		"file_name":   file.Filename,
	})
}

// processImportJob processes the import file in background
func (s *Server) processImportJob(jobID, filePath, listID string, hasHeader bool, delimiter string) {
	log.Printf("processImportJob STARTED: jobID=%s, listID=%s, filePath=%s", jobID, listID, filePath)

	// Recover from panics
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Import job %s: PANIC recovered: %v", jobID, r)
			importJobsMu.Lock()
			if job, ok := importJobs[jobID]; ok {
				job.Status = "failed"
				job.Error = fmt.Sprintf("Panic: %v", r)
			}
			importJobsMu.Unlock()
		}
	}()

	importJobsMu.Lock()
	job := importJobs[jobID]
	// Verify ListID in job matches what we received
	log.Printf("processImportJob: job.ListID from map = %s, received listID = %s", job.ListID, listID)
	if job.ListID != listID {
		log.Printf("WARNING: ListID mismatch! job.ListID=%s != listID=%s. Correcting...", job.ListID, listID)
		job.ListID = listID
	}
	job.Status = "processing"
	importJobsMu.Unlock()

	defer func() {
		// Clean up file after processing
		os.Remove(filePath)
		log.Printf("Import job %s: cleaned up file %s", jobID, filePath)
	}()

	// Open file
	f, err := os.Open(filePath)
	if err != nil {
		importJobsMu.Lock()
		job.Status = "failed"
		job.Error = "Falha ao abrir arquivo: " + err.Error()
		importJobsMu.Unlock()
		return
	}
	defer f.Close()

	log.Printf("Import job %s: loading existing emails for list %s", jobID, listID)

	// Get existing emails for duplicate check - use a simpler query that's faster
	existingEmails := make(map[string]bool)
	rows, err := s.db.Query(`SELECT email FROM emails WHERE list_id = $1`, listID)
	if err != nil {
		log.Printf("Import job %s: error querying existing emails: %v", jobID, err)
	} else {
		count := 0
		for rows.Next() {
			var email string
			rows.Scan(&email)
			existingEmails[strings.ToLower(email)] = true
			count++
		}
		rows.Close()
		log.Printf("Import job %s: loaded %d existing emails", jobID, count)
	}

	log.Printf("Import job %s: loading blacklist", jobID)

	// Get blacklist
	blacklisted := make(map[string]bool)
	blRows, err := s.db.Query(`SELECT email FROM blacklist`)
	if err != nil {
		log.Printf("Import job %s: error querying blacklist: %v", jobID, err)
	} else {
		count := 0
		for blRows.Next() {
			var email string
			blRows.Scan(&email)
			blacklisted[strings.ToLower(email)] = true
			count++
		}
		blRows.Close()
		log.Printf("Import job %s: loaded %d blacklisted emails", jobID, count)
	}

	log.Printf("Import job %s: starting file processing", jobID)

	// Process file
	scanner := bufio.NewScanner(f)
	const maxCapacity = 1024 * 1024
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	lineNum := 0
	validCount := 0
	invalidCount := 0
	duplicateCount := 0

	const batchSize = 5000
	batchCount := 0

	tx, err := s.db.Begin()
	if err != nil {
		importJobsMu.Lock()
		job.Status = "failed"
		job.Error = "Falha ao iniciar transacao"
		importJobsMu.Unlock()
		return
	}

	stmt, err := tx.Prepare(`INSERT INTO emails (id, list_id, email, name, valid) VALUES ($1, $2, $3, $4, true) ON CONFLICT (list_id, email) DO NOTHING`)
	if err != nil {
		tx.Rollback()
		importJobsMu.Lock()
		job.Status = "failed"
		job.Error = "Falha ao preparar statement"
		importJobsMu.Unlock()
		return
	}

	log.Printf("Import job %s: starting processing, total lines: %d", jobID, job.TotalLines)

	lastProgressLog := time.Now()

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

		// Get email (first column)
		var email, name string
		email = strings.TrimSpace(parts[0])
		if len(parts) > 1 {
			name = strings.TrimSpace(parts[1])
		}

		// Validate email
		email = strings.ToLower(email)
		if !emailRegex.MatchString(email) {
			invalidCount++
		} else if blacklisted[email] {
			// Check blacklist
			invalidCount++
		} else if existingEmails[email] {
			// Check duplicate
			duplicateCount++
		} else {
			// Insert email
			emailID := uuid.New().String()
			_, err := stmt.Exec(emailID, listID, email, name)

			if err == nil {
				validCount++
				existingEmails[email] = true
				batchCount++

				// Commit every batchSize valid emails
				if batchCount >= batchSize {
					stmt.Close()
					if err := tx.Commit(); err != nil {
						log.Printf("Import job %s: commit error: %v", jobID, err)
					}

					log.Printf("Import job %s: committed batch, processed %d/%d, valid %d, invalid %d, dups %d",
						jobID, lineNum, job.TotalLines, validCount, invalidCount, duplicateCount)

					// Start new transaction
					tx, err = s.db.Begin()
					if err != nil {
						log.Printf("Import job %s: failed to begin new transaction: %v", jobID, err)
						break
					}
					stmt, err = tx.Prepare(`INSERT INTO emails (id, list_id, email, name, valid) VALUES ($1, $2, $3, $4, true) ON CONFLICT (list_id, email) DO NOTHING`)
					if err != nil {
						tx.Rollback()
						log.Printf("Import job %s: failed to prepare statement: %v", jobID, err)
						break
					}
					batchCount = 0
				}
			} else {
				// Log insert errors (might be duplicate key)
				if lineNum < 100 || lineNum%10000 == 0 {
					log.Printf("Import job %s: insert error at line %d: %v", jobID, lineNum, err)
				}
				invalidCount++
			}
		}

		// Update progress every 5000 lines (regardless of valid/invalid)
		if lineNum%5000 == 0 {
			importJobsMu.Lock()
			job.Processed = lineNum
			job.Valid = validCount
			job.Invalid = invalidCount
			job.Duplicates = duplicateCount
			importJobsMu.Unlock()

			// Log progress every 10 seconds
			if time.Since(lastProgressLog) > 10*time.Second {
				log.Printf("Import job %s: progress %d/%d (%.1f%%), valid: %d, invalid: %d, dups: %d",
					jobID, lineNum, job.TotalLines, float64(lineNum)*100/float64(job.TotalLines),
					validCount, invalidCount, duplicateCount)
				lastProgressLog = time.Now()
			}
		}
	}

	// Check for scanner errors
	if err := scanner.Err(); err != nil {
		log.Printf("Import job %s: scanner error: %v", jobID, err)
		importJobsMu.Lock()
		job.Status = "failed"
		job.Error = "Erro ao ler arquivo: " + err.Error()
		importJobsMu.Unlock()
		stmt.Close()
		tx.Rollback()
		return
	}

	// Commit remaining
	stmt.Close()
	if err := tx.Commit(); err != nil {
		log.Printf("Import job %s: final commit error: %v", jobID, err)
	}

	log.Printf("Import job %s: file processing complete, updating database...", jobID)

	// Update list stats
	_, err = s.db.Exec(`
		UPDATE email_lists SET
			total_emails = total_emails + $1,
			valid_emails = valid_emails + $2,
			invalid_emails = invalid_emails + $3,
			status = 'ready'
		WHERE id = $4
	`, validCount+invalidCount, validCount, invalidCount, listID)
	if err != nil {
		log.Printf("Import job %s: error updating list stats: %v", jobID, err)
	}

	// Mark job as completed
	importJobsMu.Lock()
	job.Status = "completed"
	job.Processed = lineNum
	job.Valid = validCount
	job.Invalid = invalidCount
	job.Duplicates = duplicateCount
	job.CompletedAt = time.Now()
	importJobsMu.Unlock()

	log.Printf("Import job %s COMPLETED: %d lines processed, %d valid, %d invalid, %d duplicates", jobID, lineNum, validCount, invalidCount, duplicateCount)
}

// getImportStatus returns the status of an import job
func (s *Server) getImportStatus(c *fiber.Ctx) error {
	jobID := c.Params("jobId")

	importJobsMu.RLock()
	job, exists := importJobs[jobID]
	importJobsMu.RUnlock()

	if !exists {
		return c.Status(404).JSON(fiber.Map{"error": "Job nao encontrado"})
	}

	progress := 0
	if job.TotalLines > 0 {
		progress = (job.Processed * 100) / job.TotalLines
	}

	return c.JSON(fiber.Map{
		"id":          job.ID,
		"status":      job.Status,
		"file_name":   job.FileName,
		"total_lines": job.TotalLines,
		"processed":   job.Processed,
		"progress":    progress,
		"valid":       job.Valid,
		"invalid":     job.Invalid,
		"duplicates":  job.Duplicates,
		"error":       job.Error,
		"started_at":  job.StartedAt,
		"completed_at": job.CompletedAt,
	})
}

// getListImportJobs returns active import jobs for a list
func (s *Server) getListImportJobs(c *fiber.Ctx) error {
	listID := c.Params("id")

	importJobsMu.RLock()
	defer importJobsMu.RUnlock()

	var jobs []fiber.Map
	for _, job := range importJobs {
		log.Printf("getListImportJobs: checking job %s for list %s, job.ListID=%s, job.Status=%s", job.ID, listID, job.ListID, job.Status)
		if job.ListID == listID && (job.Status == "pending" || job.Status == "processing") {
			progress := 0
			if job.TotalLines > 0 {
				progress = (job.Processed * 100) / job.TotalLines
			}
			jobs = append(jobs, fiber.Map{
				"id":          job.ID,
				"status":      job.Status,
				"file_name":   job.FileName,
				"total_lines": job.TotalLines,
				"processed":   job.Processed,
				"progress":    progress,
				"valid":       job.Valid,
				"invalid":     job.Invalid,
				"duplicates":  job.Duplicates,
			})
		}
	}

	if jobs == nil {
		jobs = []fiber.Map{}
	}

	log.Printf("getListImportJobs: returning %d active jobs for list %s", len(jobs), listID)
	return c.JSON(fiber.Map{"jobs": jobs})
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
