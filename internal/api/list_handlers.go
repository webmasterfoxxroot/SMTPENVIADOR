package api

import (
	"bufio"
	"context"
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
	"github.com/lib/pq"
	"smtpenviador/internal/clickhouse"
)

type EmailListRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

// detectEmailColumn detects which column contains email addresses
// First checks header names, then scans data to find valid emails
func detectEmailColumn(line string, delimiter string, isHeader bool) int {
	parts := strings.Split(line, delimiter)

	// If this is a header line, check column names
	if isHeader {
		emailHeaders := []string{"email", "e-mail", "e_mail", "mail", "emailaddress", "email_address", "correo", "endereco_email"}
		for i, part := range parts {
			cleaned := strings.ToLower(strings.TrimSpace(part))
			// Remove quotes if present
			cleaned = strings.Trim(cleaned, "\"'")
			for _, header := range emailHeaders {
				if cleaned == header {
					return i
				}
			}
		}
	}

	// Scan for column containing valid email
	for i, part := range parts {
		cleaned := strings.ToLower(strings.TrimSpace(part))
		// Remove quotes if present
		cleaned = strings.Trim(cleaned, "\"'")
		if emailRegex.MatchString(cleaned) {
			return i
		}
	}

	// Default to first column
	return 0
}

// parseCSVLine handles CSV parsing with quoted fields
func parseCSVLine(line string, delimiter string) []string {
	// Simple case: no quotes
	if !strings.Contains(line, "\"") {
		return strings.Split(line, delimiter)
	}

	// Handle quoted fields
	var parts []string
	var current strings.Builder
	inQuotes := false

	for i := 0; i < len(line); i++ {
		char := line[i]
		if char == '"' {
			inQuotes = !inQuotes
		} else if string(char) == delimiter && !inQuotes {
			parts = append(parts, current.String())
			current.Reset()
		} else {
			current.WriteByte(char)
		}
	}
	parts = append(parts, current.String())

	return parts
}

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
	userID := getUserID(c)
	rows, err := s.db.Query(`
		SELECT id, name, description, total_emails, valid_emails,
		       invalid_emails, status, group_id, created_at, updated_at
		FROM email_lists
		WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch email lists"})
	}
	defer rows.Close()

	var lists []fiber.Map
	var listIDs []string

	for rows.Next() {
		var id, name, status string
		var description, groupID *string
		var totalEmails, validEmails, invalidEmails int
		var createdAt, updatedAt time.Time

		err := rows.Scan(&id, &name, &description, &totalEmails, &validEmails,
			&invalidEmails, &status, &groupID, &createdAt, &updatedAt)
		if err != nil {
			continue
		}

		listIDs = append(listIDs, id)
		lists = append(lists, fiber.Map{
			"id":             id,
			"name":           name,
			"description":    description,
			"total_emails":   totalEmails,
			"valid_emails":   validEmails,
			"invalid_emails": invalidEmails,
			"status":         status,
			"group_id":       groupID,
			"created_at":     createdAt,
			"updated_at":     updatedAt,
		})
	}

	// Update counts from ClickHouse if available (async, non-blocking)
	if s.ch != nil && len(listIDs) > 0 {
		go func() {
			ctx := context.Background()
			for i, listID := range listIDs {
				count, err := s.ch.GetEmailCountByList(ctx, listID)
				if err == nil && count > 0 {
					s.db.Exec(`UPDATE email_lists SET total_emails = $1, valid_emails = $1 WHERE id = $2`, count, listID)
					// Update the response data if we have it
					if i < len(lists) {
						lists[i]["total_emails"] = count
						lists[i]["valid_emails"] = count
					}
				}
			}
		}()

		// Also get counts synchronously for the response
		ctx := context.Background()
		for i, listID := range listIDs {
			count, err := s.ch.GetEmailCountByList(ctx, listID)
			if err == nil && count > 0 {
				lists[i]["total_emails"] = count
				lists[i]["valid_emails"] = count
			}
		}
	}

	return c.JSON(fiber.Map{
		"data":  lists,
		"total": len(lists),
	})
}

// createEmailList creates a new email list
func (s *Server) createEmailList(c *fiber.Ctx) error {
	userID := getUserID(c)
	var req EmailListRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Name is required"})
	}

	id := uuid.New().String()

	_, err := s.db.Exec(`
		INSERT INTO email_lists (id, name, description, status, user_id)
		VALUES ($1, $2, $3, 'ready', $4)
	`, id, req.Name, req.Description, userID)

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
	userID := getUserID(c)
	id := c.Params("id")

	var name, status string
	var description *string
	var totalEmails, validEmails, invalidEmails int
	var createdAt, updatedAt time.Time

	err := s.db.QueryRow(`
		SELECT name, description, total_emails, valid_emails,
		       invalid_emails, status, created_at, updated_at
		FROM email_lists WHERE id = $1 AND user_id = $2
	`, id, userID).Scan(&name, &description, &totalEmails, &validEmails,
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
	userID := getUserID(c)
	id := c.Params("id")

	var req EmailListRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	result, err := s.db.Exec(`
		UPDATE email_lists SET name = $1, description = $2 WHERE id = $3 AND user_id = $4
	`, req.Name, req.Description, id, userID)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update email list"})
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "Email list not found"})
	}

	return c.JSON(fiber.Map{"message": "Email list updated"})
}

// refreshListCounts updates email counts from ClickHouse for all lists
func (s *Server) refreshListCounts(c *fiber.Ctx) error {
	if s.ch == nil {
		return c.Status(400).JSON(fiber.Map{"error": "ClickHouse not available"})
	}

	ctx := context.Background()

	// Get all lists
	listRows, err := s.db.Query(`SELECT id FROM email_lists`)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to get lists"})
	}
	defer listRows.Close()

	updatedCount := 0
	for listRows.Next() {
		var listID string
		if err := listRows.Scan(&listID); err != nil {
			continue
		}

		// Get count from ClickHouse
		count, err := s.ch.GetEmailCountByList(ctx, listID)
		if err != nil {
			log.Printf("Error getting count for list %s: %v", listID, err)
			continue
		}

		// Update in PostgreSQL
		_, err = s.db.Exec(`
			UPDATE email_lists SET total_emails = $1, valid_emails = $1 WHERE id = $2
		`, count, listID)
		if err != nil {
			log.Printf("Error updating count for list %s: %v", listID, err)
			continue
		}

		if count > 0 {
			log.Printf("Updated list %s with %d emails", listID, count)
			updatedCount++
		}
	}

	return c.JSON(fiber.Map{
		"message": fmt.Sprintf("%d listas atualizadas", updatedCount),
		"updated": updatedCount,
	})
}

// deleteEmailList deletes an email list and all its emails (in batches for large lists)
func (s *Server) deleteEmailList(c *fiber.Ctx) error {
	userID := getUserID(c)
	id := c.Params("id")

	// Check if list exists and belongs to user
	var exists bool
	err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM email_lists WHERE id = $1 AND user_id = $2)`, id, userID).Scan(&exists)
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
	result, err := s.db.Exec(`DELETE FROM email_lists WHERE id = $1 AND user_id = $2`, id, userID)
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

	totalDeleted := 0

	// Delete from ClickHouse if available
	if s.ch != nil {
		ctx := context.Background()
		count, err := s.ch.GetEmailCountByList(ctx, listID)
		if err == nil {
			totalDeleted = int(count)
			err = s.ch.DeleteEmailsByList(ctx, listID)
			if err != nil {
				log.Printf("ClickHouse delete failed for list %s: %v, falling back to PostgreSQL", listID, err)
			} else {
				log.Printf("List %s: deleted %d emails from ClickHouse", listID, totalDeleted)
			}
		}
	}

	// Also delete from PostgreSQL (for consistency)
	// OPTIMIZED: Use direct DELETE with CTID for fast batch deletion
	const batchSize = 50000

	for {
		// Delete a batch of emails using ctid (much faster than subquery)
		result, err := s.db.Exec(`
			DELETE FROM emails
			WHERE ctid IN (
				SELECT ctid FROM emails WHERE list_id = $1 LIMIT $2
			)
		`, listID, batchSize)

		if err != nil {
			// Fallback to simple delete if ctid doesn't work
			log.Printf("Batch delete with ctid failed, trying direct delete: %v", err)
			result, err = s.db.Exec(`DELETE FROM emails WHERE list_id = $1`, listID)
			if err != nil {
				log.Printf("Direct delete also failed for list %s: %v", listID, err)
				break
			}
			deleted, _ := result.RowsAffected()
			if totalDeleted == 0 {
				totalDeleted = int(deleted)
			}
			break
		}

		deleted, _ := result.RowsAffected()
		if totalDeleted == 0 {
			totalDeleted += int(deleted)
		}

		log.Printf("List %s: deleted batch of %d emails from PostgreSQL", listID, deleted)

		if deleted < int64(batchSize) {
			// No more emails to delete
			break
		}
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
	userID := getUserID(c)
	id := c.Params("id")

	// Check if list exists and belongs to user
	var exists bool
	var status string
	err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM email_lists WHERE id = $1 AND user_id = $2)`, id, userID).Scan(&exists)
	if err != nil || !exists {
		return c.Status(404).JSON(fiber.Map{"error": "Email list not found"})
	}

	// Get list status
	s.db.QueryRow(`SELECT status FROM email_lists WHERE id = $1 AND user_id = $2`, id, userID).Scan(&status)

	log.Printf("Force deleting list %s (status: %s)", id, status)

	var emailsDeleted int64

	// Delete from ClickHouse first if available
	if s.ch != nil {
		ctx := context.Background()
		count, err := s.ch.GetEmailCountByList(ctx, id)
		if err == nil {
			emailsDeleted = int64(count)
			err = s.ch.DeleteEmailsByList(ctx, id)
			if err != nil {
				log.Printf("Force delete - ClickHouse delete failed for list %s: %v", id, err)
			} else {
				log.Printf("Force delete - deleted %d emails from ClickHouse for list %s", emailsDeleted, id)
			}
		}
	}

	// Also delete from PostgreSQL for consistency
	result, err := s.db.Exec(`DELETE FROM emails WHERE list_id = $1`, id)
	if err != nil {
		log.Printf("Force delete - failed to delete emails from PostgreSQL for list %s: %v", id, err)
		return c.Status(500).JSON(fiber.Map{"error": "Falha ao deletar emails"})
	}

	pgDeleted, _ := result.RowsAffected()
	if emailsDeleted == 0 {
		emailsDeleted = pgDeleted
	}
	log.Printf("Force delete - deleted %d emails from PostgreSQL for list %s", pgDeleted, id)

	// Delete import jobs
	s.db.Exec(`DELETE FROM import_jobs WHERE list_id = $1`, id)

	// Delete the list
	_, err = s.db.Exec(`DELETE FROM email_lists WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		log.Printf("Force delete - failed to delete list %s: %v", id, err)
		return c.Status(500).JSON(fiber.Map{"error": "Falha ao deletar lista"})
	}

	log.Printf("Force delete - successfully deleted list %s with %d emails", id, emailsDeleted)
	return c.JSON(fiber.Map{
		"message":        "Lista excluida com sucesso",
		"emails_deleted": emailsDeleted,
	})
}

// uploadEmails handles CSV/TXT file upload
func (s *Server) uploadEmails(c *fiber.Ctx) error {
	userID := getUserID(c)
	id := c.Params("id")

	// Verify list belongs to user
	var exists bool
	s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM email_lists WHERE id = $1 AND user_id = $2)`, id, userID).Scan(&exists)
	if !exists {
		return c.Status(404).JSON(fiber.Map{"error": "List not found"})
	}

	// Get file
	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "No file uploaded"})
	}

	// Get column mappings (auto = auto-detect)
	emailCol := c.FormValue("email_column", "auto")
	nameCol := c.FormValue("name_column", "")
	delimiter := c.FormValue("delimiter", ",")
	hasHeader := c.FormValue("has_header", "true") == "true"
	autoDetectEmail := emailCol == "auto" || emailCol == ""

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

	// Check blacklist - use ClickHouse if available
	blacklisted := make(map[string]bool)
	if s.ch != nil {
		ctx := context.Background()
		entries, _, err := s.ch.SearchBlacklist(ctx, "", 10000000, 0)
		if err == nil {
			for _, entry := range entries {
				blacklisted[strings.ToLower(entry.Email)] = true
			}
			log.Printf("Loaded %d blacklisted emails from ClickHouse", len(blacklisted))
		}
	}
	// Fallback to PostgreSQL if ClickHouse not available or failed
	if len(blacklisted) == 0 {
		blRows, _ := s.db.Query(`SELECT email FROM blacklist`)
		for blRows.Next() {
			var email string
			blRows.Scan(&email)
			blacklisted[strings.ToLower(email)] = true
		}
		blRows.Close()
	}

	// Parse column indexes (will be updated if auto-detect)
	emailIdx := 0
	if !autoDetectEmail {
		emailIdx = parseColIndex(emailCol)
	}
	nameIdx := -1
	if nameCol != "" {
		nameIdx = parseColIndex(nameCol)
	}

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

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Auto-detect email column from first line
		if lineNum == 1 && autoDetectEmail {
			emailIdx = detectEmailColumn(line, delimiter, hasHeader)
			log.Printf("Auto-detected email column at index %d", emailIdx)
		}

		// Skip header
		if lineNum == 1 && hasHeader {
			continue
		}

		// Parse line (handles quoted CSV fields)
		parts := parseCSVLine(line, delimiter)
		if len(parts) == 0 {
			continue
		}

		// Get email from detected/specified column
		var email, name string
		if emailIdx < len(parts) {
			email = strings.TrimSpace(parts[emailIdx])
			email = strings.Trim(email, "\"'")
		}
		if nameIdx >= 0 && nameIdx < len(parts) {
			name = strings.TrimSpace(parts[nameIdx])
			name = strings.Trim(name, "\"'")
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
	userID := getUserID(c)
	listID := c.Params("id")

	// Verify list belongs to user
	var exists bool
	s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM email_lists WHERE id = $1 AND user_id = $2)`, listID, userID).Scan(&exists)
	if !exists {
		return c.Status(404).JSON(fiber.Map{"error": "List not found"})
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

// processImportJobsSequentially processes import jobs one after another
func (s *Server) processImportJobsSequentially(jobIDs []string) {
	log.Printf("processImportJobsSequentially: starting %d jobs", len(jobIDs))
	for i, jobID := range jobIDs {
		log.Printf("processImportJobsSequentially: processing job %d/%d: %s", i+1, len(jobIDs), jobID)
		s.processImportJobDB(jobID)
		log.Printf("processImportJobsSequentially: finished job %d/%d: %s", i+1, len(jobIDs), jobID)
	}
	log.Printf("processImportJobsSequentially: all %d jobs completed", len(jobIDs))
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

	// OPTIMIZED: No memory tracking - let database handle ALL duplicates
	log.Printf("Import job %s: no memory tracking - database handles duplicates", jobID)

	// Load blacklist - use ClickHouse if available for faster lookups
	blacklisted := make(map[string]bool)
	if s.ch != nil {
		ctx := context.Background()
		// Get all blacklisted emails from ClickHouse
		entries, _, err := s.ch.SearchBlacklist(ctx, "", 10000000, 0) // Get all
		if err != nil {
			log.Printf("Import job %s: ClickHouse blacklist error: %v, falling back to PostgreSQL", jobID, err)
		} else {
			for _, entry := range entries {
				blacklisted[strings.ToLower(entry.Email)] = true
			}
			log.Printf("Import job %s: loaded %d blacklisted emails from ClickHouse", jobID, len(blacklisted))
		}
	}

	// Fallback to PostgreSQL blacklist if ClickHouse not available or failed
	if len(blacklisted) == 0 {
		blRows, err := s.db.Query(`SELECT email FROM blacklist`)
		if err != nil {
			log.Printf("Import job %s: error querying PostgreSQL blacklist: %v", jobID, err)
		} else {
			for blRows.Next() {
				var email string
				blRows.Scan(&email)
				blacklisted[strings.ToLower(email)] = true
			}
			blRows.Close()
			log.Printf("Import job %s: loaded %d blacklisted emails from PostgreSQL", jobID, len(blacklisted))
		}
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

	// OPTIMIZED: Batch COPY settings - read from database config
	batchSize := 50000 // Default value - COPY can handle much more
	batchSizeStr := s.getSettingValue("import_batch_size", "50000")
	if bs, err := strconv.Atoi(batchSizeStr); err == nil && bs > 0 && bs <= 100000 {
		batchSize = bs
	}
	log.Printf("Import job %s: using COPY with batch size of %d emails", jobID, batchSize)
	type emailRecord struct {
		id    string
		email string
		name  string
	}
	batch := make([]emailRecord, 0, batchSize)

	// Function to flush batch - uses ClickHouse if available, falls back to PostgreSQL
	// Returns: (duplicates found in this batch, error)
	flushBatch := func() (int, error) {
		if len(batch) == 0 {
			return 0, nil
		}

		batchCount := len(batch)
		var dupsInBatch int

		// Try ClickHouse first if available
		if s.ch != nil {
			ctx := context.Background()
			// Convert to ClickHouse EmailEntry format
			chEntries := make([]clickhouse.EmailEntry, len(batch))
			for i, record := range batch {
				chEntries[i] = clickhouse.EmailEntry{
					ID:     record.id,
					ListID: listID,
					Email:  record.email,
					Name:   record.name,
					Valid:  true,
				}
			}

			inserted, err := s.ch.InsertEmailsBatch(ctx, listID, chEntries)
			if err != nil {
				log.Printf("Import job %s: ClickHouse batch insert error: %v, falling back to PostgreSQL", jobID, err)
			} else {
				log.Printf("Import job %s: inserted %d emails into ClickHouse", jobID, inserted)
				batch = batch[:0] // Clear batch
				return 0, nil     // ClickHouse doesn't return duplicate count, assume 0
			}
		}

		// Fallback to PostgreSQL
		tx, err := s.db.Begin()
		if err != nil {
			return 0, err
		}

		// Create temp table (unlogged for speed)
		_, err = tx.Exec(`CREATE TEMP TABLE temp_import (
			id VARCHAR(36),
			list_id VARCHAR(36),
			email VARCHAR(255),
			name VARCHAR(255),
			valid BOOLEAN
		) ON COMMIT DROP`)
		if err != nil {
			tx.Rollback()
			return 0, fmt.Errorf("create temp table: %w", err)
		}

		// Use pq.CopyIn for fast bulk insert
		stmt, err := tx.Prepare(pq.CopyIn("temp_import", "id", "list_id", "email", "name", "valid"))
		if err != nil {
			tx.Rollback()
			return 0, fmt.Errorf("prepare copy: %w", err)
		}

		for _, record := range batch {
			_, err = stmt.Exec(record.id, listID, record.email, record.name, true)
			if err != nil {
				stmt.Close()
				tx.Rollback()
				return 0, fmt.Errorf("copy exec: %w", err)
			}
		}

		// Flush COPY data
		_, err = stmt.Exec()
		if err != nil {
			stmt.Close()
			tx.Rollback()
			return 0, fmt.Errorf("copy flush: %w", err)
		}
		stmt.Close()

		// Move from temp to real table with ON CONFLICT (cast to uuid)
		result, err := tx.Exec(`INSERT INTO emails (id, list_id, email, name, valid)
			SELECT id::uuid, list_id::uuid, email, name, valid FROM temp_import
			ON CONFLICT (list_id, email) DO NOTHING`)
		if err != nil {
			tx.Rollback()
			return 0, fmt.Errorf("insert from temp: %w", err)
		}

		inserted, _ := result.RowsAffected()
		dupsInBatch = batchCount - int(inserted)

		err = tx.Commit()
		if err != nil {
			return 0, fmt.Errorf("commit: %w", err)
		}

		batch = batch[:0] // Clear batch
		return dupsInBatch, nil
	}

	log.Printf("Import job %s: starting processing, total lines: %d", jobID, totalLines)

	lastProgressLog := time.Now()
	lastProgressUpdate := time.Now()

	// Auto-detect email column from first line
	emailColIdx := 0

	for scanner.Scan() {
		lineNum++

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// First line: detect email column
		if lineNum == 1 {
			emailColIdx = detectEmailColumn(line, delimiter, hasHeader)
			log.Printf("Import job %s: detected email column at index %d", jobID, emailColIdx)

			// Skip if this is a header line
			if hasHeader {
				continue
			}
		}

		// Parse line (handles quoted CSV fields)
		parts := parseCSVLine(line, delimiter)
		if len(parts) == 0 {
			continue
		}

		// Get email from detected column
		var email, name string
		if emailColIdx < len(parts) {
			email = strings.TrimSpace(parts[emailColIdx])
			// Remove quotes if present
			email = strings.Trim(email, "\"'")
		}
		// Try to get name from adjacent column if exists
		nameColIdx := emailColIdx + 1
		if nameColIdx < len(parts) {
			name = strings.TrimSpace(parts[nameColIdx])
			name = strings.Trim(name, "\"'")
		}

		// Validate email
		email = strings.ToLower(email)
		if !emailRegex.MatchString(email) {
			invalidCount++
		} else if blacklisted[email] {
			invalidCount++
		} else {
			// Add to batch - database handles duplicates with ON CONFLICT
			batch = append(batch, emailRecord{
				id:    uuid.New().String(),
				email: email,
				name:  name,
			})
			validCount++

			// Flush batch when full
			if len(batch) >= batchSize {
				batchLen := len(batch)
				dups, err := flushBatch()
				if err != nil {
					log.Printf("Import job %s: error flushing batch: %v", jobID, err)
				}
				duplicateCount += dups
				inserted := batchLen - dups

				// Update list email count incrementally (faster than COUNT)
				s.db.Exec(`UPDATE email_lists SET total_emails = total_emails + $1 WHERE id = $2`, inserted, listID)
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
	finalBatchLen := len(batch)
	dups, err := flushBatch()
	if err != nil {
		log.Printf("Import job %s: error flushing final batch: %v", jobID, err)
	}
	duplicateCount += dups
	finalInserted := finalBatchLen - dups
	s.db.Exec(`UPDATE email_lists SET total_emails = total_emails + $1 WHERE id = $2`, finalInserted, listID)

	// Check for scanner errors
	if err := scanner.Err(); err != nil {
		log.Printf("Import job %s: scanner error: %v", jobID, err)
		updateJobStatus("failed", "Erro ao ler arquivo: "+err.Error())
		return
	}

	log.Printf("Import job %s: file processing complete, updating database...", jobID)

	// Update list stats - get count from ClickHouse if available, otherwise from PostgreSQL
	var totalEmailCount int64
	if s.ch != nil {
		ctx := context.Background()
		count, err := s.ch.GetEmailCountByList(ctx, listID)
		if err == nil {
			totalEmailCount = int64(count)
			log.Printf("Import job %s: got %d emails from ClickHouse", jobID, totalEmailCount)
		}
	}

	// If ClickHouse count is 0, try PostgreSQL
	if totalEmailCount == 0 {
		s.db.QueryRow(`SELECT COUNT(*) FROM emails WHERE list_id = $1 AND valid = true`, listID).Scan(&totalEmailCount)
	}

	// Update list stats with the actual count
	_, err = s.db.Exec(`
		UPDATE email_lists SET
			total_emails = $1,
			valid_emails = $1,
			invalid_emails = $2,
			status = 'ready'
		WHERE id = $3
	`, totalEmailCount, invalidCount, listID)
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

// uploadEmailsSplit handles large file uploads with automatic splitting into multiple lists
func (s *Server) uploadEmailsSplit(c *fiber.Ctx) error {
	userID := getUserID(c)

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
			"error": fmt.Sprintf("Arquivo muito grande. Maximo permitido: %d MB", maxSizeMB),
		})
	}

	// Get options
	baseName := c.FormValue("base_name", "Lista")
	numPartsStr := c.FormValue("num_parts", "5")
	numParts, _ := strconv.Atoi(numPartsStr)
	if numParts < 2 {
		numParts = 2
	}
	if numParts > 50 {
		numParts = 50
	}
	hasHeader := c.FormValue("has_header", "false") == "true"
	delimiter := c.FormValue("delimiter", ",")
	groupID := c.FormValue("group_id", "")

	log.Printf("Split upload: baseName=%s, numParts=%d, hasHeader=%v, groupID=%s", baseName, numParts, hasHeader, groupID)

	// Create uploads directory
	uploadDir := "/tmp/smtpenviador/uploads"
	os.MkdirAll(uploadDir, 0755)

	// Save file to disk first
	mainFileID := uuid.New().String()
	ext := filepath.Ext(file.Filename)
	savedPath := filepath.Join(uploadDir, fmt.Sprintf("%s%s", mainFileID, ext))

	src, err := file.Open()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Falha ao abrir arquivo"})
	}

	dst, err := os.Create(savedPath)
	if err != nil {
		src.Close()
		return c.Status(500).JSON(fiber.Map{"error": "Falha ao salvar arquivo"})
	}

	_, err = io.Copy(dst, src)
	src.Close()
	dst.Close()
	if err != nil {
		os.Remove(savedPath)
		return c.Status(500).JSON(fiber.Map{"error": "Falha ao copiar arquivo"})
	}

	log.Printf("Split upload: file saved to %s", savedPath)

	// Count total lines
	f, err := os.Open(savedPath)
	if err != nil {
		os.Remove(savedPath)
		return c.Status(500).JSON(fiber.Map{"error": "Falha ao ler arquivo"})
	}

	scanner := bufio.NewScanner(f)
	const maxCapacity = 1024 * 1024
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	totalLines := 0
	for scanner.Scan() {
		totalLines++
	}
	f.Close()

	if totalLines == 0 {
		os.Remove(savedPath)
		return c.Status(400).JSON(fiber.Map{"error": "Arquivo vazio"})
	}

	// Subtract header if present
	dataLines := totalLines
	if hasHeader {
		dataLines--
	}

	linesPerPart := (dataLines + numParts - 1) / numParts // Round up
	log.Printf("Split upload: totalLines=%d, dataLines=%d, linesPerPart=%d", totalLines, dataLines, linesPerPart)

	// Create lists and split file
	type jobInfo struct {
		ListID   string `json:"list_id"`
		JobID    string `json:"job_id"`
		FilePath string `json:"-"`
		FileName string `json:"-"`
		Lines    int    `json:"-"`
	}
	jobs := make([]jobInfo, 0, numParts)

	// Re-open file for splitting
	f, err = os.Open(savedPath)
	if err != nil {
		os.Remove(savedPath)
		return c.Status(500).JSON(fiber.Map{"error": "Falha ao reabrir arquivo"})
	}
	defer f.Close()

	scanner = bufio.NewScanner(f)
	scanner.Buffer(buf, maxCapacity)

	lineNum := 0
	var header string

	// Read header if present
	if hasHeader && scanner.Scan() {
		header = scanner.Text()
		lineNum++
	}

	currentPart := 0
	currentPartLines := 0
	var currentFile *os.File
	var currentPath string
	var currentListID string

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Start new part if needed
		if currentFile == nil || currentPartLines >= linesPerPart {
			// Close previous file and save info for later job creation
			if currentFile != nil {
				currentFile.Close()
				// Save job info for later
				jobs = append(jobs, jobInfo{
					ListID:   currentListID,
					FilePath: currentPath,
					FileName: fmt.Sprintf("%s_%02d%s", baseName, currentPart, ext),
					Lines:    currentPartLines,
				})
				log.Printf("Split upload: finished part %d with %d lines", currentPart, currentPartLines)
			}

			currentPart++
			if currentPart > numParts {
				break
			}

			// Create new list
			listName := fmt.Sprintf("%s %02d", baseName, currentPart)
			currentListID = uuid.New().String()
			if groupID != "" {
				_, err = s.db.Exec(`
					INSERT INTO email_lists (id, name, description, total_emails, valid_emails, invalid_emails, status, group_id, user_id)
					VALUES ($1, $2, $3, 0, 0, 0, 'pending', $4, $5)
				`, currentListID, listName, fmt.Sprintf("Parte %d de %d", currentPart, numParts), groupID, userID)
			} else {
				_, err = s.db.Exec(`
					INSERT INTO email_lists (id, name, description, total_emails, valid_emails, invalid_emails, status, user_id)
					VALUES ($1, $2, $3, 0, 0, 0, 'pending', $4)
				`, currentListID, listName, fmt.Sprintf("Parte %d de %d", currentPart, numParts), userID)
			}
			if err != nil {
				log.Printf("Failed to create list %s: %v", listName, err)
				continue
			}

			// Create file for this part
			currentPath = filepath.Join(uploadDir, fmt.Sprintf("%s_part%d%s", mainFileID, currentPart, ext))
			currentFile, err = os.Create(currentPath)
			if err != nil {
				log.Printf("Failed to create part file: %v", err)
				continue
			}

			// Write header if present
			if header != "" {
				currentFile.WriteString(header + "\n")
			}

			currentPartLines = 0
			log.Printf("Split upload: created list %s, file %s", listName, currentPath)
		}

		// Write line to current file
		if currentFile != nil {
			currentFile.WriteString(line + "\n")
			currentPartLines++
		}
	}

	// Close last part
	if currentFile != nil {
		currentFile.Close()
		jobs = append(jobs, jobInfo{
			ListID:   currentListID,
			FilePath: currentPath,
			FileName: fmt.Sprintf("%s_%02d%s", baseName, currentPart, ext),
			Lines:    currentPartLines,
		})
		log.Printf("Split upload: finished part %d with %d lines", currentPart, currentPartLines)
	}

	// Delete main file
	os.Remove(savedPath)

	log.Printf("Split upload: creating %d import jobs...", len(jobs))

	// Create all import jobs and collect job IDs
	var jobIDs []string
	for i, job := range jobs {
		jobID := uuid.New().String()
		_, err = s.db.Exec(`
			INSERT INTO import_jobs (id, list_id, file_name, file_path, status, has_header, delimiter, total_lines, processed, valid, invalid, duplicates)
			VALUES ($1, $2, $3, $4, 'pending', $5, $6, $7, 0, 0, 0, 0)
		`, jobID, job.ListID, job.FileName, job.FilePath, false, delimiter, job.Lines)
		if err != nil {
			log.Printf("Failed to create import job for part %d: %v", i+1, err)
			// Mark list as failed
			s.db.Exec(`UPDATE email_lists SET status = 'failed' WHERE id = $1`, job.ListID)
			continue
		}
		job.JobID = jobID
		jobs[i] = job
		jobIDs = append(jobIDs, jobID)
		log.Printf("Split upload: created job %s for list %s", jobID, job.ListID)
	}

	log.Printf("Split upload complete: created %d import jobs", len(jobIDs))

	// Start processing jobs sequentially in background
	if len(jobIDs) > 0 {
		go s.processImportJobsSequentially(jobIDs)
	}

	return c.JSON(fiber.Map{
		"message": fmt.Sprintf("%d listas criadas", len(jobIDs)),
		"jobs":    jobs,
	})
}

// getListEmails returns emails from a list
func (s *Server) getListEmails(c *fiber.Ctx) error {
	userID := getUserID(c)
	id := c.Params("id")
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 50)
	offset := (page - 1) * limit

	// Verify list belongs to user
	var exists bool
	s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM email_lists WHERE id = $1 AND user_id = $2)`, id, userID).Scan(&exists)
	if !exists {
		return c.Status(404).JSON(fiber.Map{"error": "List not found"})
	}

	// Use ClickHouse if available
	if s.ch != nil {
		ctx := context.Background()
		entries, total, err := s.ch.GetEmailsByList(ctx, id, limit, offset)
		if err != nil {
			log.Printf("[ListEmails] ClickHouse error: %v, falling back to PostgreSQL", err)
		} else {
			var emails []fiber.Map
			for _, e := range entries {
				emails = append(emails, fiber.Map{
					"id":           e.ID,
					"email":        e.Email,
					"name":         e.Name,
					"custom1":      e.Custom1,
					"custom2":      e.Custom2,
					"custom3":      e.Custom3,
					"valid":        e.Valid,
					"bounced":      e.Bounced,
					"unsubscribed": e.Unsubscribed,
					"created_at":   e.CreatedAt,
				})
			}
			return c.JSON(fiber.Map{
				"data":  emails,
				"total": total,
				"page":  page,
				"limit": limit,
			})
		}
	}

	// Fallback to PostgreSQL
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
	userID := getUserID(c)
	listID := c.Params("id")
	emailID := c.Params("emailId")

	// Verify list belongs to user
	var exists bool
	s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM email_lists WHERE id = $1 AND user_id = $2)`, listID, userID).Scan(&exists)
	if !exists {
		return c.Status(404).JSON(fiber.Map{"error": "List not found"})
	}

	// Try to delete from ClickHouse first
	if s.ch != nil {
		ctx := context.Background()
		s.ch.DeleteEmail(ctx, emailID)
	}

	// Also delete from PostgreSQL (for fallback/legacy)
	result, err := s.db.Exec(`DELETE FROM emails WHERE id = $1 AND list_id = $2`, emailID, listID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete email"})
	}

	rows, _ := result.RowsAffected()

	// Update list count (decrement)
	s.db.Exec(`UPDATE email_lists SET total_emails = total_emails - 1, valid_emails = valid_emails - 1 WHERE id = $1`, listID)

	// Refresh count from ClickHouse if available
	if s.ch != nil {
		ctx := context.Background()
		count, err := s.ch.GetEmailCountByList(ctx, listID)
		if err == nil {
			s.db.Exec(`UPDATE email_lists SET total_emails = $1, valid_emails = $1 WHERE id = $2`, count, listID)
		}
	}

	if rows == 0 {
		return c.JSON(fiber.Map{"message": "Email deleted from ClickHouse"})
	}

	return c.JSON(fiber.Map{"message": "Email deleted"})
}

// addEmailManually adds a single email to a list
func (s *Server) addEmailManually(c *fiber.Ctx) error {
	userID := getUserID(c)
	listID := c.Params("id")

	var req struct {
		Email   string `json:"email"`
		Name    string `json:"name"`
		Custom1 string `json:"custom1"`
		Custom2 string `json:"custom2"`
		Custom3 string `json:"custom3"`
		Custom4 string `json:"custom4"`
		Custom5 string `json:"custom5"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// Validate email format
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if !emailRegex.MatchString(req.Email) {
		return c.Status(400).JSON(fiber.Map{"error": "Email inválido"})
	}

	// Verify list belongs to user
	var exists bool
	err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM email_lists WHERE id = $1 AND user_id = $2)`, listID, userID).Scan(&exists)
	if err != nil || !exists {
		return c.Status(404).JSON(fiber.Map{"error": "List not found"})
	}

	emailID := uuid.New().String()

	// Insert into ClickHouse if available
	if s.ch != nil {
		ctx := context.Background()
		email := clickhouse.EmailEntry{
			ID:      emailID,
			ListID:  listID,
			Email:   req.Email,
			Name:    req.Name,
			Custom1: req.Custom1,
			Custom2: req.Custom2,
			Custom3: req.Custom3,
			Custom4: req.Custom4,
			Custom5: req.Custom5,
			Valid:   true,
		}
		err = s.ch.InsertEmail(ctx, email)
		if err != nil {
			// Check for duplicate
			if strings.Contains(err.Error(), "duplicate") {
				return c.Status(400).JSON(fiber.Map{"error": "Email já existe nesta lista"})
			}
			log.Printf("Failed to insert email to ClickHouse: %v", err)
		}
	}

	// Also insert into PostgreSQL (for fallback/legacy)
	_, err = s.db.Exec(`
		INSERT INTO emails (id, list_id, email, name, custom1, custom2, custom3, custom4, custom5, valid)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, true)
		ON CONFLICT (list_id, email) DO NOTHING
	`, emailID, listID, req.Email, req.Name, req.Custom1, req.Custom2, req.Custom3, req.Custom4, req.Custom5)

	// Update list count
	s.db.Exec(`UPDATE email_lists SET total_emails = total_emails + 1, valid_emails = valid_emails + 1 WHERE id = $1`, listID)

	// Refresh count from ClickHouse if available
	if s.ch != nil {
		ctx := context.Background()
		count, err := s.ch.GetEmailCountByList(ctx, listID)
		if err == nil {
			s.db.Exec(`UPDATE email_lists SET total_emails = $1, valid_emails = $1 WHERE id = $2`, count, listID)
		}
	}

	return c.Status(201).JSON(fiber.Map{
		"message": "Email adicionado com sucesso",
		"id":      emailID,
	})
}

// updateEmail updates an existing email in a list
func (s *Server) updateEmail(c *fiber.Ctx) error {
	userID := getUserID(c)
	listID := c.Params("id")
	emailID := c.Params("emailId")

	var req struct {
		Email   string `json:"email"`
		Name    string `json:"name"`
		Custom1 string `json:"custom1"`
		Custom2 string `json:"custom2"`
		Custom3 string `json:"custom3"`
		Custom4 string `json:"custom4"`
		Custom5 string `json:"custom5"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// Validate email format
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if !emailRegex.MatchString(req.Email) {
		return c.Status(400).JSON(fiber.Map{"error": "Email inválido"})
	}

	// Verify list belongs to user
	var exists bool
	s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM email_lists WHERE id = $1 AND user_id = $2)`, listID, userID).Scan(&exists)
	if !exists {
		return c.Status(404).JSON(fiber.Map{"error": "List not found"})
	}

	// Update in ClickHouse if available
	if s.ch != nil {
		ctx := context.Background()
		err := s.ch.UpdateEmail(ctx, emailID, req.Email, req.Name, req.Custom1, req.Custom2, req.Custom3, req.Custom4, req.Custom5)
		if err != nil {
			log.Printf("Failed to update email in ClickHouse: %v", err)
		}
	}

	// Also update in PostgreSQL (for fallback/legacy)
	result, err := s.db.Exec(`
		UPDATE emails SET email = $1, name = $2, custom1 = $3, custom2 = $4, custom3 = $5, custom4 = $6, custom5 = $7
		WHERE id = $8 AND list_id = $9
	`, req.Email, req.Name, req.Custom1, req.Custom2, req.Custom3, req.Custom4, req.Custom5, emailID, listID)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Falha ao atualizar email"})
	}

	rows, _ := result.RowsAffected()
	if rows == 0 && s.ch == nil {
		return c.Status(404).JSON(fiber.Map{"error": "Email não encontrado"})
	}

	return c.JSON(fiber.Map{"message": "Email atualizado com sucesso"})
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
