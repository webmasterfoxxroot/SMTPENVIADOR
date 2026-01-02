package api

import (
	"bufio"
	"database/sql"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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

// resumeOrphanedImportJobs checks for import jobs that were interrupted by server restart
// and resumes them automatically
func (s *Server) resumeOrphanedImportJobs() {
	// Wait a bit for server to fully start
	time.Sleep(3 * time.Second)

	log.Println("[ImportRecovery] Checking for orphaned import jobs...")

	// Find jobs that were processing or pending when server stopped
	rows, err := s.db.Query(`
		SELECT id, list_id, file_path, status
		FROM import_jobs
		WHERE status IN ('pending', 'processing')
		ORDER BY created_at ASC
	`)
	if err != nil {
		log.Printf("[ImportRecovery] Error querying jobs: %v", err)
		return
	}
	defer rows.Close()

	var jobsToResume []struct {
		ID       string
		ListID   string
		FilePath string
		Status   string
	}

	for rows.Next() {
		var job struct {
			ID       string
			ListID   string
			FilePath string
			Status   string
		}
		if err := rows.Scan(&job.ID, &job.ListID, &job.FilePath, &job.Status); err != nil {
			continue
		}
		jobsToResume = append(jobsToResume, job)
	}

	if len(jobsToResume) == 0 {
		log.Println("[ImportRecovery] No orphaned jobs found")
	} else {
		log.Printf("[ImportRecovery] Found %d orphaned jobs to check", len(jobsToResume))

		for _, job := range jobsToResume {
			// Check if file still exists
			if _, err := os.Stat(job.FilePath); os.IsNotExist(err) {
				// File was deleted, mark job as failed
				log.Printf("[ImportRecovery] Job %s: file not found, marking as failed", job.ID)
				s.db.Exec(`
					UPDATE import_jobs SET status = 'failed', error_message = 'Arquivo nao encontrado apos reinicio', updated_at = NOW()
					WHERE id = $1
				`, job.ID)
				// Reset list status
				s.db.Exec(`UPDATE email_lists SET status = 'ready' WHERE id = $1`, job.ListID)
				continue
			}

			// File exists, resume the job
			log.Printf("[ImportRecovery] Resuming job %s for list %s", job.ID, job.ListID)

			// Reset job progress to start fresh (safer than trying to resume from middle)
			s.db.Exec(`
				UPDATE import_jobs SET status = 'pending', processed = 0, valid = 0, invalid = 0, duplicates = 0, updated_at = NOW()
				WHERE id = $1
			`, job.ID)

			// Start processing in background
			go s.processImportJobDB(job.ID)

			// Small delay between resuming multiple jobs
			time.Sleep(500 * time.Millisecond)
		}

		log.Printf("[ImportRecovery] Resumed %d import jobs", len(jobsToResume))
	}

	// Also check for lists stuck in "deleting" status
	s.resumeOrphanedDeletions()
}

// resumeOrphanedDeletions checks for lists stuck in "deleting" status and resumes deletion
func (s *Server) resumeOrphanedDeletions() {
	log.Println("[DeleteRecovery] Checking for lists stuck in deleting status...")

	rows, err := s.db.Query(`SELECT id FROM email_lists WHERE status = 'deleting'`)
	if err != nil {
		log.Printf("[DeleteRecovery] Error querying: %v", err)
		return
	}
	defer rows.Close()

	var listsToDelete []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		listsToDelete = append(listsToDelete, id)
	}

	if len(listsToDelete) == 0 {
		log.Println("[DeleteRecovery] No stuck deletions found")
		return
	}

	log.Printf("[DeleteRecovery] Found %d lists stuck in deleting, resuming...", len(listsToDelete))

	for _, listID := range listsToDelete {
		log.Printf("[DeleteRecovery] Resuming deletion for list %s", listID)
		go s.deleteListInBatches(listID)
		time.Sleep(500 * time.Millisecond)
	}
}

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

// deleteEmailList deletes an email list and all its emails (in batches for large lists)
func (s *Server) deleteEmailList(c *fiber.Ctx) error {
	id := c.Params("id")

	// Check if list exists
	var exists bool
	err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM email_lists WHERE id = $1)`, id).Scan(&exists)
	if err != nil || !exists {
		return c.Status(404).JSON(fiber.Map{"error": "Email list not found"})
	}

	// Check if any campaigns are using this list
	var campaignCount int
	s.db.QueryRow(`SELECT COUNT(*) FROM campaigns WHERE list_id = $1`, id).Scan(&campaignCount)
	if campaignCount > 0 {
		return c.Status(400).JSON(fiber.Map{
			"error": fmt.Sprintf("Nao pode excluir: %d campanha(s) usando esta lista. Exclua as campanhas primeiro.", campaignCount),
		})
	}

	// Check how many emails in this list
	var emailCount int
	s.db.QueryRow(`SELECT COUNT(*) FROM emails WHERE list_id = $1`, id).Scan(&emailCount)

	log.Printf("Deleting list %s with %d emails", id, emailCount)

	// For large lists, delete in batches to avoid timeout
	if emailCount > 10000 {
		// Mark list as deleting
		s.db.Exec(`UPDATE email_lists SET status = 'deleting' WHERE id = $1`, id)

		// Delete in background
		go s.deleteListInBatches(id)

		return c.JSON(fiber.Map{
			"message": "Exclusao iniciada em background",
			"emails":  emailCount,
		})
	}

	// For smaller lists, delete directly
	// First delete import jobs
	s.db.Exec(`DELETE FROM import_jobs WHERE list_id = $1`, id)

	// Delete all emails
	_, err = s.db.Exec(`DELETE FROM emails WHERE list_id = $1`, id)
	if err != nil {
		log.Printf("Failed to delete emails for list %s: %v", id, err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete emails"})
	}

	// Delete the list
	result, err := s.db.Exec(`DELETE FROM email_lists WHERE id = $1`, id)
	if err != nil {
		log.Printf("Failed to delete list %s: %v", id, err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete email list"})
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "Email list not found"})
	}

	log.Printf("Deleted email list %s with %d emails", id, emailCount)
	return c.JSON(fiber.Map{"message": "Email list deleted"})
}

// deleteListInBatches deletes emails in batches to avoid timeout
func (s *Server) deleteListInBatches(listID string) {
	log.Printf("Starting batch deletion for list %s", listID)

	const batchSize = 10000
	totalDeleted := 0

	for {
		// Delete a batch of emails
		result, err := s.db.Exec(`
			DELETE FROM emails
			WHERE id IN (
				SELECT id FROM emails WHERE list_id = $1 LIMIT $2
			)
		`, listID, batchSize)

		if err != nil {
			log.Printf("Batch delete error for list %s: %v", listID, err)
			break
		}

		deleted, _ := result.RowsAffected()
		totalDeleted += int(deleted)

		log.Printf("List %s: deleted batch of %d emails (total: %d)", listID, deleted, totalDeleted)

		if deleted < batchSize {
			// No more emails to delete
			break
		}

		// Small pause between batches
		time.Sleep(100 * time.Millisecond)
	}

	// Delete import jobs
	s.db.Exec(`DELETE FROM import_jobs WHERE list_id = $1`, listID)

	// Finally delete the list
	_, err := s.db.Exec(`DELETE FROM email_lists WHERE id = $1`, listID)
	if err != nil {
		log.Printf("Failed to delete list %s after batch deletion: %v", listID, err)
		return
	}

	log.Printf("Successfully deleted list %s with %d emails", listID, totalDeleted)
}

// forceDeleteEmailList forces immediate deletion of a list regardless of size
func (s *Server) forceDeleteEmailList(c *fiber.Ctx) error {
	id := c.Params("id")

	// Check if list exists
	var exists bool
	var status string
	err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM email_lists WHERE id = $1)`, id).Scan(&exists)
	if err != nil || !exists {
		return c.Status(404).JSON(fiber.Map{"error": "Email list not found"})
	}

	// Get list status
	s.db.QueryRow(`SELECT status FROM email_lists WHERE id = $1`, id).Scan(&status)

	log.Printf("Force deleting list %s (status: %s)", id, status)

	// Delete all emails directly (this may take a while for large lists)
	result, err := s.db.Exec(`DELETE FROM emails WHERE list_id = $1`, id)
	if err != nil {
		log.Printf("Force delete - failed to delete emails for list %s: %v", id, err)
		return c.Status(500).JSON(fiber.Map{"error": "Falha ao deletar emails"})
	}

	emailsDeleted, _ := result.RowsAffected()
	log.Printf("Force delete - deleted %d emails from list %s", emailsDeleted, id)

	// Delete import jobs
	s.db.Exec(`DELETE FROM import_jobs WHERE list_id = $1`, id)

	// Delete the list
	_, err = s.db.Exec(`DELETE FROM email_lists WHERE id = $1`, id)
	if err != nil {
		log.Printf("Force delete - failed to delete list %s: %v", id, err)
		return c.Status(500).JSON(fiber.Map{"error": "Falha ao deletar lista"})
	}

	log.Printf("Force delete - successfully deleted list %s with %d emails", id, emailsDeleted)
	return c.JSON(fiber.Map{
		"message": "Lista excluida com sucesso",
		"emails_deleted": emailsDeleted,
	})
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
// Jobs are stored in database for persistence across page changes and restarts
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
	hasHeader := c.FormValue("has_header", "false") == "true"
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

	// Copy file to disk
	written, err := io.Copy(dst, src)
	dst.Close()
	if err != nil {
		os.Remove(savedPath)
		return c.Status(500).JSON(fiber.Map{"error": "Falha ao copiar arquivo"})
	}

	log.Printf("File saved: %s (%d bytes)", savedPath, written)

	// Count lines in file for progress tracking
	countFile, _ := os.Open(savedPath)
	lineCount := 0
	scanner := bufio.NewScanner(countFile)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		lineCount++
	}
	countFile.Close()
	if hasHeader && lineCount > 0 {
		lineCount--
	}

	// Create import job in DATABASE (not in-memory!)
	_, err = s.db.Exec(`
		INSERT INTO import_jobs (id, list_id, file_name, file_path, status, total_lines, has_header, delimiter, started_at)
		VALUES ($1, $2, $3, $4, 'pending', $5, $6, $7, NOW())
	`, jobID, listID, file.Filename, savedPath, lineCount, hasHeader, delimiter)
	if err != nil {
		os.Remove(savedPath)
		log.Printf("Failed to create import job: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Falha ao criar job de importacao"})
	}

	log.Printf("Import job created in DB: ID=%s, ListID=%s, TotalLines=%d", jobID, listID, lineCount)

	// Update list status
	s.db.Exec(`UPDATE email_lists SET status = 'importing' WHERE id = $1`, listID)

	// Start background processing
	go s.processImportJobDB(jobID)

	return c.JSON(fiber.Map{
		"message":     "Upload iniciado",
		"job_id":      jobID,
		"total_lines": lineCount,
		"file_name":   file.Filename,
	})
}

// processImportJobDB processes the import file in background using database for state
// OPTIMIZED: Uses batch INSERT for much faster import (10-50x faster)
func (s *Server) processImportJobDB(jobID string) {
	log.Printf("processImportJobDB STARTED: jobID=%s", jobID)

	// Helper to update job status in database
	updateJobStatus := func(status string, errorMsg string) {
		if errorMsg != "" {
			s.db.Exec(`UPDATE import_jobs SET status = $1, error_message = $2, updated_at = NOW() WHERE id = $3`,
				status, errorMsg, jobID)
		} else {
			s.db.Exec(`UPDATE import_jobs SET status = $1, updated_at = NOW() WHERE id = $2`, status, jobID)
		}
	}

	// Helper to update job progress in database
	updateJobProgress := func(processed, valid, invalid, duplicates int) {
		s.db.Exec(`UPDATE import_jobs SET processed = $1, valid = $2, invalid = $3, duplicates = $4, updated_at = NOW() WHERE id = $5`,
			processed, valid, invalid, duplicates, jobID)
	}

	// Helper to check if job was cancelled
	isJobCancelled := func() bool {
		var status string
		err := s.db.QueryRow(`SELECT status FROM import_jobs WHERE id = $1`, jobID).Scan(&status)
		return err != nil || status == "cancelled"
	}

	// Recover from panics
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Import job %s: PANIC recovered: %v", jobID, r)
			updateJobStatus("failed", fmt.Sprintf("Panic: %v", r))
		}
	}()

	// Read job details from database
	var listID, filePath, delimiter string
	var hasHeader bool
	var totalLines int
	err := s.db.QueryRow(`
		SELECT list_id, file_path, has_header, delimiter, total_lines
		FROM import_jobs WHERE id = $1
	`, jobID).Scan(&listID, &filePath, &hasHeader, &delimiter, &totalLines)
	if err != nil {
		log.Printf("Import job %s: failed to read job from database: %v", jobID, err)
		updateJobStatus("failed", "Job nao encontrado no banco")
		return
	}

	log.Printf("processImportJobDB: jobID=%s, listID=%s, filePath=%s, totalLines=%d", jobID, listID, filePath, totalLines)

	// Update status to processing
	updateJobStatus("processing", "")

	defer func() {
		// Clean up file after processing
		os.Remove(filePath)
		log.Printf("Import job %s: cleaned up file %s", jobID, filePath)
	}()

	// Open file
	f, err := os.Open(filePath)
	if err != nil {
		updateJobStatus("failed", "Falha ao abrir arquivo: "+err.Error())
		return
	}
	defer f.Close()

	log.Printf("Import job %s: loading existing emails for list %s", jobID, listID)

	// Get existing emails for duplicate check
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

	log.Printf("Import job %s: starting OPTIMIZED file processing", jobID)

	// Process file
	scanner := bufio.NewScanner(f)
	const maxCapacity = 1024 * 1024
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	lineNum := 0
	validCount := 0
	invalidCount := 0
	duplicateCount := 0

	// OPTIMIZED: Batch INSERT settings - read from database config
	batchSize := 1000 // Default value
	batchSizeStr := s.getSettingValue("import_batch_size", "1000")
	if bs, err := strconv.Atoi(batchSizeStr); err == nil && bs > 0 && bs <= 10000 {
		batchSize = bs
	}
	log.Printf("Import job %s: using batch size of %d emails per INSERT", jobID, batchSize)
	type emailRecord struct {
		id    string
		email string
		name  string
	}
	batch := make([]emailRecord, 0, batchSize)

	// Function to flush batch to database
	flushBatch := func() error {
		if len(batch) == 0 {
			return nil
		}

		// Build multi-value INSERT query
		valueStrings := make([]string, 0, len(batch))
		valueArgs := make([]interface{}, 0, len(batch)*4)
		for i, record := range batch {
			valueStrings = append(valueStrings, fmt.Sprintf("($%d, $%d, $%d, $%d, true)", i*4+1, i*4+2, i*4+3, i*4+4))
			valueArgs = append(valueArgs, record.id, listID, record.email, record.name)
		}

		query := fmt.Sprintf(
			"INSERT INTO emails (id, list_id, email, name, valid) VALUES %s ON CONFLICT (list_id, email) DO NOTHING",
			strings.Join(valueStrings, ","),
		)

		_, err := s.db.Exec(query, valueArgs...)
		if err != nil {
			log.Printf("Import job %s: batch insert error: %v", jobID, err)
			return err
		}

		batch = batch[:0] // Clear batch
		return nil
	}

	log.Printf("Import job %s: starting processing, total lines: %d", jobID, totalLines)

	lastProgressLog := time.Now()
	lastProgressUpdate := time.Now()

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
			invalidCount++
		} else if existingEmails[email] {
			duplicateCount++
		} else {
			// Add to batch
			batch = append(batch, emailRecord{
				id:    uuid.New().String(),
				email: email,
				name:  name,
			})
			existingEmails[email] = true
			validCount++

			// Flush batch when full
			if len(batch) >= batchSize {
				if err := flushBatch(); err != nil {
					log.Printf("Import job %s: error flushing batch: %v", jobID, err)
				}
			}
		}

		// Update progress every 10000 lines or every 2 seconds
		if lineNum%10000 == 0 || time.Since(lastProgressUpdate) > 2*time.Second {
			updateJobProgress(lineNum, validCount, invalidCount, duplicateCount)
			lastProgressUpdate = time.Now()

			// Check if job was cancelled by user
			if isJobCancelled() {
				log.Printf("Import job %s: CANCELLED by user at line %d", jobID, lineNum)
				s.db.Exec(`UPDATE email_lists SET status = 'ready' WHERE id = $1`, listID)
				return
			}

			// Log progress every 10 seconds
			if time.Since(lastProgressLog) > 10*time.Second {
				speed := float64(lineNum) / time.Since(lastProgressLog).Seconds() * 10
				log.Printf("Import job %s: progress %d/%d (%.1f%%), valid: %d, invalid: %d, dups: %d, speed: %.0f/s",
					jobID, lineNum, totalLines, float64(lineNum)*100/float64(totalLines),
					validCount, invalidCount, duplicateCount, speed)
				lastProgressLog = time.Now()
			}
		}
	}

	// Flush remaining batch
	if err := flushBatch(); err != nil {
		log.Printf("Import job %s: error flushing final batch: %v", jobID, err)
	}

	// Check for scanner errors
	if err := scanner.Err(); err != nil {
		log.Printf("Import job %s: scanner error: %v", jobID, err)
		updateJobStatus("failed", "Erro ao ler arquivo: "+err.Error())
		return
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

	// Mark job as completed in database
	s.db.Exec(`
		UPDATE import_jobs SET
			status = 'completed',
			processed = $1,
			valid = $2,
			invalid = $3,
			duplicates = $4,
			completed_at = NOW(),
			updated_at = NOW()
		WHERE id = $5
	`, lineNum, validCount, invalidCount, duplicateCount, jobID)

	log.Printf("Import job %s COMPLETED: %d lines processed, %d valid, %d invalid, %d duplicates", jobID, lineNum, validCount, invalidCount, duplicateCount)
}

// getImportStatus returns the status of an import job from database
func (s *Server) getImportStatus(c *fiber.Ctx) error {
	jobID := c.Params("jobId")

	var status, fileName string
	var errorMsg sql.NullString
	var totalLines, processed, valid, invalid, duplicates int
	var startedAt sql.NullTime
	var completedAt sql.NullTime

	err := s.db.QueryRow(`
		SELECT status, file_name, total_lines, processed, valid, invalid, duplicates,
		       error_message, started_at, completed_at
		FROM import_jobs WHERE id = $1
	`, jobID).Scan(&status, &fileName, &totalLines, &processed, &valid, &invalid, &duplicates,
		&errorMsg, &startedAt, &completedAt)

	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Job nao encontrado"})
	}

	progress := 0
	if totalLines > 0 {
		progress = (processed * 100) / totalLines
	}

	result := fiber.Map{
		"id":          jobID,
		"status":      status,
		"file_name":   fileName,
		"total_lines": totalLines,
		"processed":   processed,
		"progress":    progress,
		"valid":       valid,
		"invalid":     invalid,
		"duplicates":  duplicates,
	}

	if errorMsg.Valid {
		result["error"] = errorMsg.String
	}
	if startedAt.Valid {
		result["started_at"] = startedAt.Time
	}
	if completedAt.Valid {
		result["completed_at"] = completedAt.Time
	}

	return c.JSON(result)
}

// getListImportJobs returns active import jobs for a list from database
func (s *Server) getListImportJobs(c *fiber.Ctx) error {
	listID := c.Params("id")

	rows, err := s.db.Query(`
		SELECT id, status, file_name, total_lines, processed, valid, invalid, duplicates
		FROM import_jobs
		WHERE list_id = $1 AND status IN ('pending', 'processing')
		ORDER BY created_at DESC
	`, listID)
	if err != nil {
		log.Printf("getListImportJobs: error querying: %v", err)
		return c.JSON(fiber.Map{"jobs": []fiber.Map{}})
	}
	defer rows.Close()

	var jobs []fiber.Map
	for rows.Next() {
		var jobID, status, fileName string
		var totalLines, processed, valid, invalid, duplicates int

		err := rows.Scan(&jobID, &status, &fileName, &totalLines, &processed, &valid, &invalid, &duplicates)
		if err != nil {
			continue
		}

		progress := 0
		if totalLines > 0 {
			progress = (processed * 100) / totalLines
		}

		jobs = append(jobs, fiber.Map{
			"id":          jobID,
			"status":      status,
			"file_name":   fileName,
			"total_lines": totalLines,
			"processed":   processed,
			"progress":    progress,
			"valid":       valid,
			"invalid":     invalid,
			"duplicates":  duplicates,
		})
	}

	if jobs == nil {
		jobs = []fiber.Map{}
	}

	log.Printf("getListImportJobs: returning %d active jobs for list %s", len(jobs), listID)
	return c.JSON(fiber.Map{"jobs": jobs})
}

// cancelImportJob cancels an import job in progress
func (s *Server) cancelImportJob(c *fiber.Ctx) error {
	jobID := c.Params("jobId")

	// Get job info
	var status, listID, filePath string
	err := s.db.QueryRow(`SELECT status, list_id, file_path FROM import_jobs WHERE id = $1`, jobID).Scan(&status, &listID, &filePath)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Job nao encontrado"})
	}

	// Can only cancel pending or processing jobs
	if status != "pending" && status != "processing" {
		return c.Status(400).JSON(fiber.Map{"error": "Job ja foi finalizado"})
	}

	log.Printf("Cancelling import job %s (status: %s)", jobID, status)

	// Mark as cancelled in database - the processing goroutine will check this
	_, err = s.db.Exec(`
		UPDATE import_jobs SET status = 'cancelled', error_message = 'Cancelado pelo usuario', updated_at = NOW()
		WHERE id = $1
	`, jobID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Falha ao cancelar job"})
	}

	// Reset list status
	s.db.Exec(`UPDATE email_lists SET status = 'ready' WHERE id = $1`, listID)

	// Try to delete the uploaded file
	if filePath != "" {
		os.Remove(filePath)
	}

	log.Printf("Import job %s cancelled successfully", jobID)
	return c.JSON(fiber.Map{"message": "Importacao cancelada"})
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
