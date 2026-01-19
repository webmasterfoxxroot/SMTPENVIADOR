package api

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"smtpenviador/internal/clickhouse"
	"smtpenviador/internal/queue"
)

// getTrackingDomain returns the tracking domain from settings (global fallback)
func (s *Server) getTrackingDomain() string {
	var domain string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = 'tracking_domain'`).Scan(&domain)
	if err != nil {
		return s.cfg.TrackingDomain // Fallback to config
	}
	return domain
}

// getTrackingDomainForUser returns the tracking domain for a specific user
func (s *Server) getTrackingDomainForUser(userID string) string {
	var domain sql.NullString
	err := s.db.QueryRow(`SELECT tracking_domain FROM users WHERE id = $1`, userID).Scan(&domain)
	if err == nil && domain.Valid && domain.String != "" {
		return domain.String
	}
	// Fallback to global settings
	return s.getTrackingDomain()
}

// getTrackingDomainForCampaign returns the tracking domain for a campaign's owner
func (s *Server) getTrackingDomainForCampaign(campaignID string) string {
	var domain sql.NullString
	err := s.db.QueryRow(`
		SELECT u.tracking_domain FROM users u
		JOIN campaigns c ON c.user_id = u.id
		WHERE c.id = $1
	`, campaignID).Scan(&domain)
	if err == nil && domain.Valid && domain.String != "" {
		return domain.String
	}
	// Fallback to global settings
	return s.getTrackingDomain()
}

// ensureCampaignEmailsUpdated ensures the campaign_emails table has the email column
func (s *Server) ensureCampaignEmailsUpdated() {
	// Check if email column exists
	var exists bool
	s.db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'campaign_emails' AND column_name = 'email'
		)
	`).Scan(&exists)

	if !exists {
		log.Println("[Migration] Adding email column to campaign_emails table...")
		_, err := s.db.Exec(`ALTER TABLE campaign_emails ADD COLUMN email VARCHAR(255)`)
		if err != nil {
			log.Printf("[Migration] Warning: Could not add email column: %v", err)
		} else {
			// Also add name column for display
			s.db.Exec(`ALTER TABLE campaign_emails ADD COLUMN IF NOT EXISTS recipient_name VARCHAR(255)`)
			log.Println("[Migration] Added email and recipient_name columns to campaign_emails")
		}
	}
}

type CampaignRequest struct {
	Name        string     `json:"name"`
	Subject     string     `json:"subject"`
	FromName    string     `json:"from_name"`
	FromEmail   string     `json:"from_email"`
	ReplyTo     string     `json:"reply_to"`
	HTMLContent string     `json:"html_content"`
	TextContent string     `json:"text_content"`
	ListID      string     `json:"list_id"`  // For backwards compatibility
	ListIDs     []string   `json:"list_ids"` // Multiple lists support
	SmtpIDs     []string   `json:"smtp_ids"` // Multiple SMTPs support
	SendRate    int        `json:"send_rate"`
	Threads     int        `json:"threads"`  // Number of parallel threads/workers (1-100)
	ScheduledAt *time.Time `json:"scheduled_at"`
	TrackOpens  bool       `json:"track_opens"`
	TrackClicks bool       `json:"track_clicks"`
}

// listCampaigns returns all campaigns
func (s *Server) listCampaigns(c *fiber.Ctx) error {
	userID := getUserID(c)
	status := c.Query("status", "")

	// Use EXTRACT(EPOCH FROM ...) to get Unix timestamp directly from PostgreSQL
	// This avoids timezone conversion issues between PostgreSQL and Go
	query := `
		SELECT c.id, c.name, c.subject, c.from_name, c.from_email, c.status,
		       c.total_emails, c.sent_count, c.failed_count, c.open_count,
		       c.click_count, c.bounce_count, c.scheduled_at,
		       CASE WHEN c.auto_start_at IS NOT NULL THEN EXTRACT(EPOCH FROM c.auto_start_at)::bigint ELSE NULL END as auto_start_at_unix,
		       c.started_at, c.completed_at, c.created_at, l.name as list_name
		FROM campaigns c
		LEFT JOIN email_lists l ON c.list_id = l.id
		WHERE c.user_id = $1
	`
	args := []interface{}{userID}

	if status != "" {
		query += " AND c.status = $2"
		args = append(args, status)
	}

	query += " ORDER BY c.created_at DESC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch campaigns"})
	}
	defer rows.Close()

	var campaigns []fiber.Map
	for rows.Next() {
		var id, name, subject, fromName, fromEmail, campaignStatus string
		var listName *string
		var totalEmails, sentCount, failedCount, openCount, clickCount, bounceCount int
		var scheduledAt, startedAt, completedAt *time.Time
		var autoStartAtUnix *int64
		var createdAt time.Time

		err := rows.Scan(&id, &name, &subject, &fromName, &fromEmail, &campaignStatus,
			&totalEmails, &sentCount, &failedCount, &openCount, &clickCount, &bounceCount,
			&scheduledAt, &autoStartAtUnix, &startedAt, &completedAt, &createdAt, &listName)
		if err != nil {
			fmt.Printf("[listCampaigns] Scan error: %v\n", err)
			continue
		}

		campaigns = append(campaigns, fiber.Map{
			"id":            id,
			"name":          name,
			"subject":       subject,
			"from_name":     fromName,
			"from_email":    fromEmail,
			"status":        campaignStatus,
			"total_emails":  totalEmails,
			"sent_count":    sentCount,
			"failed_count":  failedCount,
			"open_count":    openCount,
			"click_count":   clickCount,
			"bounce_count":  bounceCount,
			"scheduled_at":  scheduledAt,
			"auto_start_at": autoStartAtUnix,
			"started_at":    startedAt,
			"completed_at":  completedAt,
			"created_at":    createdAt,
			"list_name":     listName,
		})
	}

	return c.JSON(fiber.Map{
		"data":        campaigns,
		"total":       len(campaigns),
		"server_time": time.Now().Unix(), // For frontend to sync clocks
	})
}

// createCampaign creates a new campaign (frontend will handle countdown and start)
func (s *Server) createCampaign(c *fiber.Ctx) error {
	userID := getUserID(c)

	var req CampaignRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// Validate - from_email not required anymore (comes from SMTP senders)
	if req.Name == "" || req.Subject == "" || req.HTMLContent == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Missing required fields"})
	}

	// Support both list_id (single) and list_ids (multiple)
	listIDs := req.ListIDs
	if len(listIDs) == 0 && req.ListID != "" {
		listIDs = []string{req.ListID}
	}

	if len(listIDs) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "At least one list is required"})
	}

	// Handle SMTP selection - if smtp_ids provided, validate them; otherwise use all active SMTPs
	smtpIDs := req.SmtpIDs
	var smtpCount int

	if len(smtpIDs) > 0 {
		// Validate that provided SMTPs belong to user and are active
		placeholders := make([]string, len(smtpIDs))
		args := make([]interface{}, len(smtpIDs)+1)
		args[0] = userID
		for i, id := range smtpIDs {
			placeholders[i] = fmt.Sprintf("$%d", i+2)
			args[i+1] = id
		}
		query := fmt.Sprintf(`SELECT COUNT(*) FROM smtp_servers WHERE active = true AND user_id = $1 AND id IN (%s)`, strings.Join(placeholders, ","))
		s.db.QueryRow(query, args...).Scan(&smtpCount)

		if smtpCount == 0 {
			return c.Status(400).JSON(fiber.Map{"error": "Nenhum dos SMTPs selecionados está ativo."})
		}
		if smtpCount != len(smtpIDs) {
			return c.Status(400).JSON(fiber.Map{"error": "Alguns SMTPs selecionados são inválidos ou inativos."})
		}
	} else {
		// No SMTPs selected - use all active SMTPs for this user
		s.db.QueryRow(`SELECT COUNT(*) FROM smtp_servers WHERE active = true AND user_id = $1`, userID).Scan(&smtpCount)
		if smtpCount == 0 {
			return c.Status(400).JSON(fiber.Map{"error": "Nenhum SMTP ativo disponível. Adicione um SMTP antes de criar campanhas."})
		}
	}

	smtpIDsStr := strings.Join(smtpIDs, ",")

	// Get email count from all selected lists
	var totalEmails int
	listIDsStr := strings.Join(listIDs, ",")

	// Try ClickHouse first for email count, fallback to PostgreSQL
	if s.ch != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		count, err := s.ch.GetEmailCountForCampaign(ctx, listIDs)
		if err == nil {
			totalEmails = int(count)
		} else {
			log.Printf("[Campaign] ClickHouse email count failed (using PostgreSQL): %v", err)
		}
	}
	if totalEmails == 0 {
		// Fallback to PostgreSQL - use LEFT JOIN for better performance
		placeholders := make([]string, len(listIDs))
		args := make([]interface{}, len(listIDs))
		for i, id := range listIDs {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
			args[i] = id
		}
		query := fmt.Sprintf(`
			SELECT COUNT(*) FROM emails e
			LEFT JOIN blacklist b ON LOWER(e.email) = LOWER(b.email)
			WHERE e.list_id IN (%s) AND e.valid = true AND e.bounced = false AND e.unsubscribed = false AND b.email IS NULL
		`, strings.Join(placeholders, ","))
		s.db.QueryRow(query, args...).Scan(&totalEmails)
	}

	id := uuid.New().String()

	// Validate threads (1-100, default 10)
	threads := req.Threads
	if threads <= 0 {
		threads = 10 // Default
	} else if threads > 100 {
		threads = 100 // Max limit
	}

	// Insert campaign as draft with auto_start_at = NOW() + 60 seconds (in UTC)
	// Use first list_id for backwards compatibility, store all in list_ids
	autoStartAt := time.Now().UTC().Add(60 * time.Second)
	_, err := s.db.Exec(`
		INSERT INTO campaigns (id, name, subject, from_name, from_email, reply_to,
		                       html_content, text_content, list_id, list_ids, smtp_ids, send_rate, threads,
		                       scheduled_at, track_opens, track_clicks, total_emails, status, auto_start_at, user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, 'draft', $18, $19)
	`, id, req.Name, req.Subject, req.FromName, req.FromEmail, req.ReplyTo,
		req.HTMLContent, req.TextContent, listIDs[0], listIDsStr, smtpIDsStr, req.SendRate, threads,
		req.ScheduledAt, req.TrackOpens, req.TrackClicks, totalEmails, autoStartAt, userID)

	if err != nil {
		log.Printf("Failed to create campaign: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create campaign"})
	}

	return c.Status(201).JSON(fiber.Map{
		"message":       "Campanha criada",
		"id":            id,
		"total_emails":  totalEmails,
		"smtp_count":    smtpCount,
		"threads":       threads,
		"auto_start_at": autoStartAt.Unix(),
	})
}

// getCampaign returns a single campaign
func (s *Server) getCampaign(c *fiber.Ctx) error {
	userID := getUserID(c)
	id := c.Params("id")

	var name, subject, fromName, fromEmail, replyTo, htmlContent, textContent, status, listID string
	var listIDsStr, smtpIDsStr sql.NullString
	var totalEmails, sentCount, failedCount, openCount, clickCount, bounceCount, sendRate, threads int
	var scheduledAt, startedAt, completedAt *time.Time
	var createdAt, updatedAt time.Time

	err := s.db.QueryRow(`
		SELECT name, subject, from_name, from_email, reply_to, html_content, text_content,
		       list_id, COALESCE(list_ids, ''), COALESCE(smtp_ids, ''), status, total_emails, sent_count, failed_count, open_count,
		       click_count, bounce_count, send_rate, COALESCE(threads, 10), scheduled_at, started_at,
		       completed_at, created_at, updated_at
		FROM campaigns WHERE id = $1 AND user_id = $2
	`, id, userID).Scan(&name, &subject, &fromName, &fromEmail, &replyTo, &htmlContent, &textContent,
		&listID, &listIDsStr, &smtpIDsStr, &status, &totalEmails, &sentCount, &failedCount, &openCount,
		&clickCount, &bounceCount, &sendRate, &threads, &scheduledAt, &startedAt,
		&completedAt, &createdAt, &updatedAt)

	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Campaign not found"})
	}

	// Parse list_ids
	var listIDs []string
	if listIDsStr.Valid && listIDsStr.String != "" {
		listIDs = strings.Split(listIDsStr.String, ",")
	} else if listID != "" {
		listIDs = []string{listID}
	}

	// Parse smtp_ids
	var smtpIDs []string
	if smtpIDsStr.Valid && smtpIDsStr.String != "" {
		smtpIDs = strings.Split(smtpIDsStr.String, ",")
	}

	return c.JSON(fiber.Map{
		"id":           id,
		"name":         name,
		"subject":      subject,
		"from_name":    fromName,
		"from_email":   fromEmail,
		"reply_to":     replyTo,
		"html_content": htmlContent,
		"text_content": textContent,
		"list_id":      listID,
		"list_ids":     listIDs,
		"smtp_ids":     smtpIDs,
		"status":       status,
		"total_emails": totalEmails,
		"sent_count":   sentCount,
		"failed_count": failedCount,
		"open_count":   openCount,
		"click_count":  clickCount,
		"bounce_count": bounceCount,
		"send_rate":    sendRate,
		"threads":      threads,
		"scheduled_at": scheduledAt,
		"started_at":   startedAt,
		"completed_at": completedAt,
		"created_at":   createdAt,
		"updated_at":   updatedAt,
	})
}

// updateCampaign updates a campaign
func (s *Server) updateCampaign(c *fiber.Ctx) error {
	userID := getUserID(c)
	id := c.Params("id")

	var req CampaignRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// Check if campaign is editable
	var status string
	s.db.QueryRow(`SELECT status FROM campaigns WHERE id = $1 AND user_id = $2`, id, userID).Scan(&status)
	if status != "draft" && status != "paused" {
		return c.Status(400).JSON(fiber.Map{"error": "Campaign cannot be edited in current status"})
	}

	// Support both list_id (single) and list_ids (multiple)
	listIDs := req.ListIDs
	if len(listIDs) == 0 && req.ListID != "" {
		listIDs = []string{req.ListID}
	}
	listIDsStr := strings.Join(listIDs, ",")
	firstListID := ""
	if len(listIDs) > 0 {
		firstListID = listIDs[0]
	}

	// Handle smtp_ids
	smtpIDsStr := strings.Join(req.SmtpIDs, ",")

	// Validate threads (1-100, default 10)
	threads := req.Threads
	if threads <= 0 {
		threads = 10 // Default
	} else if threads > 100 {
		threads = 100 // Max limit
	}

	result, err := s.db.Exec(`
		UPDATE campaigns SET
			name = $1, subject = $2, from_name = $3, from_email = $4,
			reply_to = $5, html_content = $6, text_content = $7,
			list_id = $8, list_ids = $9, smtp_ids = $10, send_rate = $11, threads = $12, scheduled_at = $13,
			track_opens = $14, track_clicks = $15
		WHERE id = $16 AND user_id = $17
	`, req.Name, req.Subject, req.FromName, req.FromEmail, req.ReplyTo,
		req.HTMLContent, req.TextContent, firstListID, listIDsStr, smtpIDsStr, req.SendRate, threads,
		req.ScheduledAt, req.TrackOpens, req.TrackClicks, id, userID)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update campaign"})
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "Campaign not found"})
	}

	return c.JSON(fiber.Map{"message": "Campaign updated"})
}

// deleteCampaign deletes a campaign
func (s *Server) deleteCampaign(c *fiber.Ctx) error {
	userID := getUserID(c)
	id := c.Params("id")

	result, err := s.db.Exec(`DELETE FROM campaigns WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete campaign"})
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "Campaign not found"})
	}

	return c.JSON(fiber.Map{"message": "Campaign deleted"})
}

// startCampaign starts sending a campaign
func (s *Server) startCampaign(c *fiber.Ctx) error {
	userID := getUserID(c)
	id := c.Params("id")

	// Get campaign details
	var listID, fromEmail, fromName, replyTo, subject, htmlContent, textContent string
	var listIDsStr sql.NullString
	var status string
	var trackOpens, trackClicks bool
	err := s.db.QueryRow(`
		SELECT list_id, COALESCE(list_ids, ''), from_email, from_name, reply_to, subject, html_content, text_content, status, COALESCE(track_opens, true), COALESCE(track_clicks, true)
		FROM campaigns WHERE id = $1 AND user_id = $2
	`, id, userID).Scan(&listID, &listIDsStr, &fromEmail, &fromName, &replyTo, &subject, &htmlContent, &textContent, &status, &trackOpens, &trackClicks)

	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Campaign not found"})
	}

	if status != "draft" && status != "paused" {
		return c.Status(400).JSON(fiber.Map{"error": "Campaign cannot be started in current status"})
	}

	// Get tracking domain for campaign owner
	trackingDomain := s.getTrackingDomainForCampaign(id)

	// Parse list IDs - use list_ids if available, otherwise use list_id
	var listIDs []string
	if listIDsStr.Valid && listIDsStr.String != "" {
		listIDs = strings.Split(listIDsStr.String, ",")
	} else {
		listIDs = []string{listID}
	}

	// Trim whitespace from list IDs
	for i := range listIDs {
		listIDs[i] = strings.TrimSpace(listIDs[i])
	}

	// Queue emails - try ClickHouse first, then PostgreSQL
	count := 0
	if s.ch != nil {
		// Use ClickHouse with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		chEmails, err := s.ch.GetEmailsForCampaign(ctx, listIDs)
		if err != nil {
			log.Printf("❌ Failed to get emails from ClickHouse: %v", err)
		} else {
			count = s.queueClickHouseEmails(chEmails, id, fromEmail, fromName, replyTo, subject, htmlContent, textContent, trackOpens, trackClicks, trackingDomain)
		}
	}

	// Fallback to PostgreSQL if no emails from ClickHouse
	if count == 0 {
		placeholders := make([]string, len(listIDs))
		args := make([]interface{}, len(listIDs))
		for i, lid := range listIDs {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
			args[i] = lid
		}

		// Use LEFT JOIN for better performance with large blacklist
		query := fmt.Sprintf(`
			SELECT e.id, e.email, e.name, e.custom1, e.custom2, e.custom3, e.custom4, e.custom5
			FROM emails e
			LEFT JOIN blacklist b ON LOWER(e.email) = LOWER(b.email)
			WHERE e.list_id IN (%s) AND e.valid = true AND e.bounced = false AND e.unsubscribed = false AND b.email IS NULL
		`, strings.Join(placeholders, ","))
		rows, err := s.db.Query(query, args...)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch emails"})
		}
		defer rows.Close()

		count = s.queueEmails(rows, id, fromEmail, fromName, replyTo, subject, htmlContent, textContent, trackOpens, trackClicks, trackingDomain)
	}

	// Update campaign status
	s.db.Exec(`
		UPDATE campaigns SET status = 'running', started_at = NOW(), total_emails = $1
		WHERE id = $2
	`, count, id)

	return c.JSON(fiber.Map{
		"message":       "Campaign started",
		"emails_queued": count,
	})
}

// pauseCampaign pauses a running campaign
func (s *Server) pauseCampaign(c *fiber.Ctx) error {
	userID := getUserID(c)
	id := c.Params("id")

	result, err := s.db.Exec(`
		UPDATE campaigns SET status = 'paused' WHERE id = $1 AND user_id = $2 AND status = 'running'
	`, id, userID)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to pause campaign"})
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Campaign is not running"})
	}

	return c.JSON(fiber.Map{"message": "Campaign paused"})
}

// resumeCampaign resumes a paused campaign
func (s *Server) resumeCampaign(c *fiber.Ctx) error {
	userID := getUserID(c)
	id := c.Params("id")

	result, err := s.db.Exec(`
		UPDATE campaigns SET status = 'running' WHERE id = $1 AND user_id = $2 AND status = 'paused'
	`, id, userID)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to resume campaign"})
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Campaign is not paused"})
	}

	// Re-queue pending emails
	count := s.requeuePendingEmails(id)
	log.Printf("[Campaign %s] Resumed and re-queued %d pending emails", id[:8], count)

	return c.JSON(fiber.Map{
		"message":  "Campaign resumed",
		"requeued": count,
	})
}

// requeueCampaign re-queues all pending emails for a running campaign
func (s *Server) requeueCampaign(c *fiber.Ctx) error {
	userID := getUserID(c)
	id := c.Params("id")

	// Verify campaign exists and is running
	var status string
	err := s.db.QueryRow(`SELECT status FROM campaigns WHERE id = $1 AND user_id = $2`, id, userID).Scan(&status)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Campaign not found"})
	}

	if status != "running" && status != "paused" {
		return c.Status(400).JSON(fiber.Map{"error": "Campaign must be running or paused to requeue"})
	}

	// Re-queue pending emails
	count := s.requeuePendingEmails(id)

	return c.JSON(fiber.Map{
		"message":  "Emails requeued",
		"requeued": count,
	})
}

// requeuePendingEmails re-queues all emails with status 'queued' for a campaign
func (s *Server) requeuePendingEmails(campaignID string) int {
	// Get campaign details
	var fromEmail, fromName, replyTo, subject, htmlContent, textContent string
	var trackOpens, trackClicks bool
	err := s.db.QueryRow(`
		SELECT from_email, from_name, reply_to, subject, html_content, text_content,
		       COALESCE(track_opens, true), COALESCE(track_clicks, true)
		FROM campaigns WHERE id = $1
	`, campaignID).Scan(&fromEmail, &fromName, &replyTo, &subject, &htmlContent, &textContent, &trackOpens, &trackClicks)
	if err != nil {
		log.Printf("❌ [Requeue] Failed to get campaign %s: %v", campaignID, err)
		return 0
	}

	// Get tracking domain for this campaign
	trackingDomain := s.getTrackingDomainForCampaign(campaignID)

	// Get all pending emails from campaign_emails
	rows, err := s.db.Query(`
		SELECT ce.id, ce.email_id, ce.email, ce.recipient_name
		FROM campaign_emails ce
		WHERE ce.campaign_id = $1 AND ce.status = 'queued'
	`, campaignID)
	if err != nil {
		log.Printf("❌ [Requeue] Failed to query pending emails: %v", err)
		return 0
	}
	defer rows.Close()

	count := 0
	batchSize := 1000
	batch := make([]*queue.EmailJob, 0, batchSize)

	for rows.Next() {
		var id, emailID, email string
		var recipientName sql.NullString
		if err := rows.Scan(&id, &emailID, &email, &recipientName); err != nil {
			continue
		}

		name := ""
		if recipientName.Valid {
			name = recipientName.String
		}

		job := &queue.EmailJob{
			ID:             id,
			CampaignID:     campaignID,
			EmailID:        emailID,
			To:             email,
			ToName:         name,
			From:           fromEmail,
			FromName:       fromName,
			ReplyTo:        replyTo,
			Subject:        subject,
			HTMLContent:    htmlContent,
			TextContent:    textContent,
			Variables:      make(map[string]string),
			TrackOpens:     trackOpens,
			TrackClicks:    trackClicks,
			TrackingDomain: trackingDomain,
			CreatedAt:      time.Now(),
		}

		batch = append(batch, job)
		count++

		// Push batch to queue
		if len(batch) >= batchSize {
			for _, j := range batch {
				s.queue.PushCampaign(j)
			}
			log.Printf("[Requeue %s] Queued %d emails...", campaignID[:8], count)
			batch = make([]*queue.EmailJob, 0, batchSize)
		}
	}

	// Push remaining batch
	for _, j := range batch {
		s.queue.PushCampaign(j)
	}

	log.Printf("[Requeue %s] Total re-queued: %d emails", campaignID[:8], count)
	return count
}

// cancelCampaign cancels a campaign
func (s *Server) cancelCampaign(c *fiber.Ctx) error {
	userID := getUserID(c)
	id := c.Params("id")

	result, err := s.db.Exec(`
		UPDATE campaigns SET status = 'cancelled' WHERE id = $1 AND user_id = $2 AND status IN ('running', 'paused', 'scheduled')
	`, id, userID)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to cancel campaign"})
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Campaign cannot be cancelled"})
	}

	// Delete pending queue items
	s.db.Exec(`DELETE FROM campaign_emails WHERE campaign_id = $1 AND status = 'queued'`, id)

	return c.JSON(fiber.Map{"message": "Campaign cancelled"})
}

// cancelAutoStart cancels the auto-start countdown for a campaign
func (s *Server) cancelAutoStart(c *fiber.Ctx) error {
	userID := getUserID(c)
	id := c.Params("id")

	result, err := s.db.Exec(`
		UPDATE campaigns SET auto_start_at = NULL WHERE id = $1 AND user_id = $2 AND status = 'draft'
	`, id, userID)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to cancel auto-start"})
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Campaign not found or not in draft status"})
	}

	return c.JSON(fiber.Map{"message": "Auto-start cancelled"})
}

// scheduleCampaign schedules a campaign to start at a specific time
func (s *Server) scheduleCampaign(c *fiber.Ctx) error {
	id := c.Params("id")

	var req struct {
		ScheduledAt string `json:"scheduled_at"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// Parse the scheduled time
	scheduledAt, err := time.Parse(time.RFC3339, req.ScheduledAt)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid date format"})
	}

	// Ensure it's in the future
	if scheduledAt.Before(time.Now()) {
		return c.Status(400).JSON(fiber.Map{"error": "Scheduled time must be in the future"})
	}

	// Update campaign with scheduled time and status
	userID := getUserID(c)
	result, err := s.db.Exec(`
		UPDATE campaigns
		SET scheduled_at = $1, status = 'scheduled', auto_start_at = NULL
		WHERE id = $2 AND user_id = $3 AND status = 'draft'
	`, scheduledAt, id, userID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to schedule campaign"})
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Campaign not found or not in draft status"})
	}

	return c.JSON(fiber.Map{
		"message":      "Campaign scheduled",
		"scheduled_at": scheduledAt.Format(time.RFC3339),
	})
}

// cancelSchedule cancels a scheduled campaign
func (s *Server) cancelSchedule(c *fiber.Ctx) error {
	userID := getUserID(c)
	id := c.Params("id")

	result, err := s.db.Exec(`
		UPDATE campaigns
		SET scheduled_at = NULL, status = 'draft'
		WHERE id = $1 AND user_id = $2 AND status = 'scheduled'
	`, id, userID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to cancel schedule"})
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Campaign not found or not scheduled"})
	}

	return c.JSON(fiber.Map{"message": "Schedule cancelled"})
}

// autoStartCampaignByID starts a campaign by ID (called by scheduler)
func (s *Server) autoStartCampaignByID(id string) {
	fmt.Printf("[AutoStart] autoStartCampaignByID called for campaign %s\n", id)

	// Get campaign details - include list_ids for multiple list support
	var listID, fromEmail, fromName, replyTo, subject, htmlContent, textContent string
	var listIDsStr sql.NullString
	var trackOpens, trackClicks bool
	err := s.db.QueryRow(`
		SELECT list_id, COALESCE(list_ids, ''), from_email, from_name, reply_to, subject, html_content, text_content, COALESCE(track_opens, true), COALESCE(track_clicks, true)
		FROM campaigns WHERE id = $1 AND status = 'draft'
	`, id).Scan(&listID, &listIDsStr, &fromEmail, &fromName, &replyTo, &subject, &htmlContent, &textContent, &trackOpens, &trackClicks)

	if err != nil {
		fmt.Printf("[AutoStart] Error getting campaign %s: %v\n", id, err)
		return
	}

	// Parse list IDs - use list_ids if available, otherwise use list_id
	var listIDs []string
	if listIDsStr.Valid && listIDsStr.String != "" {
		listIDs = strings.Split(listIDsStr.String, ",")
	} else if listID != "" {
		listIDs = []string{listID}
	}

	// Trim whitespace from list IDs
	for i := range listIDs {
		listIDs[i] = strings.TrimSpace(listIDs[i])
	}

	fmt.Printf("[AutoStart] Campaign %s - Lists: %v, From: %s\n", id, listIDs, fromEmail)

	if len(listIDs) == 0 {
		fmt.Printf("[AutoStart] Campaign %s has no lists, skipping\n", id)
		return
	}

	// Get tracking domain for campaign owner
	trackingDomain := s.getTrackingDomainForCampaign(id)

	// Queue emails - try ClickHouse first, then PostgreSQL
	count := 0

	if s.ch != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		chEmails, err := s.ch.GetEmailsForCampaign(ctx, listIDs)
		cancel()
		if err != nil {
			fmt.Printf("[AutoStart] Error getting emails from ClickHouse: %v\n", err)
		} else {
			count = s.queueClickHouseEmails(chEmails, id, fromEmail, fromName, replyTo, subject, htmlContent, textContent, trackOpens, trackClicks, trackingDomain)
		}
	}

	// Fallback to PostgreSQL
	if count == 0 {
		placeholders := make([]string, len(listIDs))
		args := make([]interface{}, len(listIDs))
		for i, lid := range listIDs {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
			args[i] = lid
		}

		// Use LEFT JOIN for better performance with large blacklist
		query := fmt.Sprintf(`
			SELECT e.id, e.email, e.name, e.custom1, e.custom2, e.custom3, e.custom4, e.custom5
			FROM emails e
			LEFT JOIN blacklist b ON LOWER(e.email) = LOWER(b.email)
			WHERE e.list_id IN (%s) AND e.valid = true AND e.bounced = false AND e.unsubscribed = false AND b.email IS NULL
		`, strings.Join(placeholders, ","))

		rows, err := s.db.Query(query, args...)
		if err != nil {
			fmt.Printf("[AutoStart] Error getting emails for campaign %s: %v\n", id, err)
			return
		}
		defer rows.Close()
		count = s.queueEmails(rows, id, fromEmail, fromName, replyTo, subject, htmlContent, textContent, trackOpens, trackClicks, trackingDomain)
	}

	fmt.Printf("[AutoStart] Queued %d emails for campaign %s\n", count, id)

	// Update campaign status
	_, err = s.db.Exec(`
		UPDATE campaigns SET status = 'running', started_at = NOW(), total_emails = $1, auto_start_at = NULL
		WHERE id = $2
	`, count, id)
	if err != nil {
		fmt.Printf("[AutoStart] Error updating campaign %s status: %v\n", id, err)
	} else {
		fmt.Printf("[AutoStart] Campaign %s started successfully!\n", id)
	}
}

// getCampaignStats returns campaign statistics
func (s *Server) getCampaignStats(c *fiber.Ctx) error {
	userID := getUserID(c)
	id := c.Params("id")

	// Verify campaign belongs to user
	var exists bool
	s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM campaigns WHERE id = $1 AND user_id = $2)`, id, userID).Scan(&exists)
	if !exists {
		return c.Status(404).JSON(fiber.Map{"error": "Campaign not found"})
	}

	// Get counts by status
	rows, err := s.db.Query(`
		SELECT status, COUNT(*) FROM campaign_emails WHERE campaign_id = $1 GROUP BY status
	`, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch stats"})
	}
	defer rows.Close()

	stats := make(map[string]int)
	for rows.Next() {
		var status string
		var count int
		rows.Scan(&status, &count)
		stats[status] = count
	}

	return c.JSON(fiber.Map{
		"queued":  stats["queued"],
		"sending": stats["sending"],
		"sent":    stats["sent"],
		"failed":  stats["failed"],
		"bounced": stats["bounced"],
	})
}

// cloneCampaign creates a copy of an existing campaign (frontend will handle countdown and start)
func (s *Server) cloneCampaign(c *fiber.Ctx) error {
	userID := getUserID(c)
	id := c.Params("id")

	// Check if there are active SMTPs available for this user
	var smtpCount int
	s.db.QueryRow(`SELECT COUNT(*) FROM smtp_servers WHERE active = true AND user_id = $1`, userID).Scan(&smtpCount)

	if smtpCount == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Nenhum SMTP ativo disponível. Adicione um SMTP antes de clonar campanhas."})
	}

	// Get original campaign
	var name, subject, fromName, fromEmail, replyTo, htmlContent, textContent, listID string
	var sendRate int
	var trackOpens, trackClicks bool
	err := s.db.QueryRow(`
		SELECT name, subject, from_name, from_email, reply_to, html_content, text_content, list_id, send_rate, COALESCE(track_opens, true), COALESCE(track_clicks, true)
		FROM campaigns WHERE id = $1 AND user_id = $2
	`, id, userID).Scan(&name, &subject, &fromName, &fromEmail, &replyTo, &htmlContent, &textContent, &listID, &sendRate, &trackOpens, &trackClicks)

	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Campaign not found"})
	}

	// Get email count from list - try ClickHouse first
	var totalEmails int
	listIDs := []string{listID}
	if s.ch != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		count, err := s.ch.GetEmailCountForCampaign(ctx, listIDs)
		cancel()
		if err == nil {
			totalEmails = int(count)
		}
	}
	if totalEmails == 0 {
		// Use LEFT JOIN for better performance
		s.db.QueryRow(`
			SELECT COUNT(*) FROM emails e
			LEFT JOIN blacklist b ON LOWER(e.email) = LOWER(b.email)
			WHERE e.list_id = $1 AND e.valid = true AND e.bounced = false AND e.unsubscribed = false AND b.email IS NULL
		`, listID).Scan(&totalEmails)
	}

	// Create new campaign with "Copy of" prefix as draft with auto_start_at = NOW() + 60 seconds (UTC)
	newID := uuid.New().String()
	newName := "Cópia de " + name
	autoStartAt := time.Now().UTC().Add(60 * time.Second)

	_, err = s.db.Exec(`
		INSERT INTO campaigns (id, name, subject, from_name, from_email, reply_to, html_content, text_content, list_id, send_rate, track_opens, track_clicks, total_emails, status, auto_start_at, user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, 'draft', $14, $15)
	`, newID, newName, subject, fromName, fromEmail, replyTo, htmlContent, textContent, listID, sendRate, trackOpens, trackClicks, totalEmails, autoStartAt, userID)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to clone campaign"})
	}

	return c.JSON(fiber.Map{
		"message":       "Campanha clonada",
		"id":            newID,
		"total_emails":  totalEmails,
		"smtp_count":    smtpCount,
		"auto_start_at": autoStartAt.Unix(),
	})
}

// resendCampaign resends the campaign to all emails in the list
func (s *Server) resendCampaign(c *fiber.Ctx) error {
	userID := getUserID(c)
	id := c.Params("id")

	// Get campaign details
	var listID, fromEmail, fromName, replyTo, subject, htmlContent, textContent string
	var trackOpens, trackClicks bool
	err := s.db.QueryRow(`
		SELECT list_id, from_email, from_name, reply_to, subject, html_content, text_content, COALESCE(track_opens, true), COALESCE(track_clicks, true)
		FROM campaigns WHERE id = $1 AND user_id = $2
	`, id, userID).Scan(&listID, &fromEmail, &fromName, &replyTo, &subject, &htmlContent, &textContent, &trackOpens, &trackClicks)

	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Campaign not found"})
	}

	// Get tracking domain for campaign owner
	trackingDomain := s.getTrackingDomainForCampaign(id)

	// Clear previous campaign_emails
	s.db.Exec(`DELETE FROM campaign_emails WHERE campaign_id = $1`, id)

	// Reset campaign counters
	s.db.Exec(`UPDATE campaigns SET sent_count = 0, failed_count = 0, open_count = 0, click_count = 0, bounce_count = 0 WHERE id = $1`, id)

	// Queue emails - try ClickHouse first, then PostgreSQL
	listIDs := []string{listID}
	count := 0

	if s.ch != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		chEmails, err := s.ch.GetEmailsForCampaign(ctx, listIDs)
		cancel()
		if err != nil {
			log.Printf("❌ Failed to get emails from ClickHouse: %v", err)
		} else {
			count = s.queueClickHouseEmails(chEmails, id, fromEmail, fromName, replyTo, subject, htmlContent, textContent, trackOpens, trackClicks, trackingDomain)
		}
	}

	// Fallback to PostgreSQL
	if count == 0 {
		// Use LEFT JOIN for better performance with large blacklist
		rows, err := s.db.Query(`
			SELECT e.id, e.email, e.name, e.custom1, e.custom2, e.custom3, e.custom4, e.custom5
			FROM emails e
			LEFT JOIN blacklist b ON LOWER(e.email) = LOWER(b.email)
			WHERE e.list_id = $1 AND e.valid = true AND e.bounced = false AND e.unsubscribed = false AND b.email IS NULL
		`, listID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch emails"})
		}
		defer rows.Close()
		count = s.queueEmails(rows, id, fromEmail, fromName, replyTo, subject, htmlContent, textContent, trackOpens, trackClicks, trackingDomain)
	}

	// Update campaign status
	s.db.Exec(`UPDATE campaigns SET status = 'running', started_at = NOW(), total_emails = $1 WHERE id = $2`, count, id)

	return c.JSON(fiber.Map{
		"message":       "Campaign resend started",
		"emails_queued": count,
	})
}

// resendToFailed resends only to emails that failed
func (s *Server) resendToFailed(c *fiber.Ctx) error {
	userID := getUserID(c)
	id := c.Params("id")

	// Get campaign details
	var fromEmail, fromName, replyTo, subject, htmlContent, textContent string
	var trackOpens, trackClicks bool
	err := s.db.QueryRow(`
		SELECT from_email, from_name, reply_to, subject, html_content, text_content, COALESCE(track_opens, true), COALESCE(track_clicks, true)
		FROM campaigns WHERE id = $1 AND user_id = $2
	`, id, userID).Scan(&fromEmail, &fromName, &replyTo, &subject, &htmlContent, &textContent, &trackOpens, &trackClicks)

	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Campaign not found"})
	}

	// Get tracking domain for campaign owner
	trackingDomain := s.getTrackingDomainForCampaign(id)

	// Get failed emails
	rows, err := s.db.Query(`
		SELECT e.id, e.email, e.name, e.custom1, e.custom2, e.custom3, e.custom4, e.custom5
		FROM emails e
		INNER JOIN campaign_emails ce ON ce.email_id = e.id
		WHERE ce.campaign_id = $1 AND ce.status = 'failed'
	`, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch failed emails"})
	}
	defer rows.Close()

	// Delete old failed records
	s.db.Exec(`DELETE FROM campaign_emails WHERE campaign_id = $1 AND status = 'failed'`, id)

	count := s.queueEmails(rows, id, fromEmail, fromName, replyTo, subject, htmlContent, textContent, trackOpens, trackClicks, trackingDomain)

	// Update campaign status
	if count > 0 {
		s.db.Exec(`UPDATE campaigns SET status = 'running' WHERE id = $1`, id)
	}

	return c.JSON(fiber.Map{
		"message":       "Resending to failed emails",
		"emails_queued": count,
	})
}

// resendToNonOpeners resends only to emails that didn't open
func (s *Server) resendToNonOpeners(c *fiber.Ctx) error {
	userID := getUserID(c)
	id := c.Params("id")

	// Get campaign details
	var fromEmail, fromName, replyTo, subject, htmlContent, textContent string
	var trackOpens, trackClicks bool
	err := s.db.QueryRow(`
		SELECT from_email, from_name, reply_to, subject, html_content, text_content, COALESCE(track_opens, true), COALESCE(track_clicks, true)
		FROM campaigns WHERE id = $1 AND user_id = $2
	`, id, userID).Scan(&fromEmail, &fromName, &replyTo, &subject, &htmlContent, &textContent, &trackOpens, &trackClicks)

	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Campaign not found"})
	}

	// Get tracking domain for campaign owner
	trackingDomain := s.getTrackingDomainForCampaign(id)

	// Get emails that were sent but not opened
	rows, err := s.db.Query(`
		SELECT e.id, e.email, e.name, e.custom1, e.custom2, e.custom3, e.custom4, e.custom5
		FROM emails e
		INNER JOIN campaign_emails ce ON ce.email_id = e.id
		WHERE ce.campaign_id = $1 AND ce.status = 'sent' AND ce.opened_at IS NULL
	`, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch non-opener emails"})
	}
	defer rows.Close()

	// Delete old sent records for non-openers
	s.db.Exec(`DELETE FROM campaign_emails WHERE campaign_id = $1 AND status = 'sent' AND opened_at IS NULL`, id)

	count := s.queueEmails(rows, id, fromEmail, fromName, replyTo, subject, htmlContent, textContent, trackOpens, trackClicks, trackingDomain)

	// Update campaign status
	if count > 0 {
		s.db.Exec(`UPDATE campaigns SET status = 'running' WHERE id = $1`, id)
	}

	return c.JSON(fiber.Map{
		"message":       "Resending to non-openers",
		"emails_queued": count,
	})
}

// queueEmails is a helper to queue emails from a rows result
// Now processes in batches for better performance
func (s *Server) queueEmails(rows *sql.Rows, campaignID, fromEmail, fromName, replyTo, subject, htmlContent, textContent string, trackOpens, trackClicks bool, trackingDomain string) int {
	count := 0
	batchSize := 1000
	batch := make([]*queue.EmailJob, 0, batchSize)

	for rows.Next() {
		var emailID, email string
		var name, c1, c2, c3, c4, c5 *string
		rows.Scan(&emailID, &email, &name, &c1, &c2, &c3, &c4, &c5)

		variables := make(map[string]string)
		if c1 != nil {
			variables["custom1"] = *c1
		}
		if c2 != nil {
			variables["custom2"] = *c2
		}
		if c3 != nil {
			variables["custom3"] = *c3
		}
		if c4 != nil {
			variables["custom4"] = *c4
		}
		if c5 != nil {
			variables["custom5"] = *c5
		}

		nameStr := ""
		if name != nil {
			nameStr = *name
		}

		job := &queue.EmailJob{
			ID:             uuid.New().String(),
			CampaignID:     campaignID,
			EmailID:        emailID,
			To:             email,
			ToName:         nameStr,
			From:           fromEmail,
			FromName:       fromName,
			ReplyTo:        replyTo,
			Subject:        subject,
			HTMLContent:    htmlContent,
			TextContent:    textContent,
			Variables:      variables,
			TrackOpens:     trackOpens,
			TrackClicks:    trackClicks,
			TrackingDomain: trackingDomain,
			CreatedAt:      time.Now(),
		}

		batch = append(batch, job)
		count++

		// Process batch when full
		if len(batch) >= batchSize {
			s.processBatch(batch, campaignID)
			log.Printf("[Campaign %s] Queued %d emails...", campaignID[:8], count)
			batch = make([]*queue.EmailJob, 0, batchSize)
		}
	}

	// Process remaining batch
	if len(batch) > 0 {
		s.processBatch(batch, campaignID)
	}

	log.Printf("[Campaign %s] Total queued: %d emails", campaignID[:8], count)
	return count
}

// processBatch processes a batch of email jobs
func (s *Server) processBatch(batch []*queue.EmailJob, campaignID string) {
	if len(batch) == 0 {
		return
	}

	// Ensure campaign_emails table has email column
	s.ensureCampaignEmailsUpdated()

	// Batch insert into campaign_emails using transaction
	tx, err := s.db.Begin()
	if err != nil {
		log.Printf("❌ Failed to start transaction: %v", err)
		return
	}

	// Include email and recipient_name in the insert
	stmt, err := tx.Prepare(`INSERT INTO campaign_emails (id, campaign_id, email_id, email, recipient_name, status) VALUES ($1, $2, $3, $4, $5, 'queued')`)
	if err != nil {
		tx.Rollback()
		log.Printf("❌ Failed to prepare statement: %v", err)
		return
	}
	defer stmt.Close()

	for _, job := range batch {
		_, err = stmt.Exec(job.ID, campaignID, job.EmailID, job.To, job.ToName)
		if err != nil {
			log.Printf("❌ Failed to insert campaign_email for %s: %v", job.To, err)
			continue
		}

		// Use dedicated CAMPAIGN queue (isolated from warmup)
		if err := s.queue.PushCampaign(job); err != nil {
			log.Printf("❌ Failed to push job to campaign queue for %s: %v", job.To, err)
		}
	}

	tx.Commit()
}

// queueClickHouseEmails is a helper to queue emails from ClickHouse results
// Now processes in batches for better performance
func (s *Server) queueClickHouseEmails(emails []clickhouse.CampaignEmail, campaignID, fromEmail, fromName, replyTo, subject, htmlContent, textContent string, trackOpens, trackClicks bool, trackingDomain string) int {
	count := 0
	batchSize := 1000
	batch := make([]*queue.EmailJob, 0, batchSize)

	for _, e := range emails {
		variables := make(map[string]string)
		if e.Custom1 != "" {
			variables["custom1"] = e.Custom1
		}
		if e.Custom2 != "" {
			variables["custom2"] = e.Custom2
		}
		if e.Custom3 != "" {
			variables["custom3"] = e.Custom3
		}
		if e.Custom4 != "" {
			variables["custom4"] = e.Custom4
		}
		if e.Custom5 != "" {
			variables["custom5"] = e.Custom5
		}

		job := &queue.EmailJob{
			ID:             uuid.New().String(),
			CampaignID:     campaignID,
			EmailID:        e.ID,
			To:             e.Email,
			ToName:         e.Name,
			From:           fromEmail,
			FromName:       fromName,
			ReplyTo:        replyTo,
			Subject:        subject,
			HTMLContent:    htmlContent,
			TextContent:    textContent,
			Variables:      variables,
			TrackOpens:     trackOpens,
			TrackClicks:    trackClicks,
			TrackingDomain: trackingDomain,
			CreatedAt:      time.Now(),
		}

		batch = append(batch, job)
		count++

		// Process batch when full
		if len(batch) >= batchSize {
			s.processBatch(batch, campaignID)
			log.Printf("[Campaign %s] Queued %d emails from ClickHouse...", campaignID[:8], count)
			batch = make([]*queue.EmailJob, 0, batchSize)
		}
	}

	// Process remaining batch
	if len(batch) > 0 {
		s.processBatch(batch, campaignID)
	}

	log.Printf("[Campaign %s] Total queued from ClickHouse: %d emails", campaignID[:8], count)
	return count
}

// exportCampaignCSV exports campaign results to CSV
func (s *Server) exportCampaignCSV(c *fiber.Ctx) error {
	userID := getUserID(c)
	id := c.Params("id")

	// Get campaign name
	var campaignName string
	err := s.db.QueryRow(`SELECT name FROM campaigns WHERE id = $1 AND user_id = $2`, id, userID).Scan(&campaignName)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Campaign not found"})
	}

	// Get all campaign emails with details - use stored email directly
	rows, err := s.db.Query(`
		SELECT ce.email, ce.recipient_name, ce.status, ce.sent_at, ce.opened_at, ce.clicked_at, ce.error_message
		FROM campaign_emails ce
		WHERE ce.campaign_id = $1
		ORDER BY ce.sent_at DESC NULLS LAST
	`, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch campaign data"})
	}
	defer rows.Close()

	// Build CSV
	var csv strings.Builder
	csv.WriteString("Email,Nome,Status,Enviado Em,Aberto Em,Clicado Em,Erro\n")

	for rows.Next() {
		var email, status string
		var name, errorMsg *string
		var sentAt, openedAt, clickedAt *time.Time

		rows.Scan(&email, &name, &status, &sentAt, &openedAt, &clickedAt, &errorMsg)

		nameStr := ""
		if name != nil {
			nameStr = *name
		}
		errorStr := ""
		if errorMsg != nil {
			errorStr = *errorMsg
		}
		sentStr := ""
		if sentAt != nil {
			sentStr = sentAt.Format("2006-01-02 15:04:05")
		}
		openedStr := ""
		if openedAt != nil {
			openedStr = openedAt.Format("2006-01-02 15:04:05")
		}
		clickedStr := ""
		if clickedAt != nil {
			clickedStr = clickedAt.Format("2006-01-02 15:04:05")
		}

		csv.WriteString(fmt.Sprintf("%s,%s,%s,%s,%s,%s,%s\n",
			email, nameStr, status, sentStr, openedStr, clickedStr, errorStr))
	}

	c.Set("Content-Type", "text/csv")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.csv\"", campaignName))
	return c.SendString(csv.String())
}

// getCampaignDetails returns detailed list of emails with status
func (s *Server) getCampaignDetails(c *fiber.Ctx) error {
	userID := getUserID(c)
	id := c.Params("id")
	status := c.Query("status", "")
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 50)
	offset := (page - 1) * limit

	// Verify campaign belongs to user
	var exists bool
	s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM campaigns WHERE id = $1 AND user_id = $2)`, id, userID).Scan(&exists)
	if !exists {
		return c.Status(404).JSON(fiber.Map{"error": "Campaign not found"})
	}

	// Build query - use stored email directly instead of JOIN
	// This supports both ClickHouse and PostgreSQL emails
	query := `
		SELECT ce.email, ce.recipient_name, ce.status, ce.sent_at, ce.opened_at, ce.clicked_at, ce.error_message
		FROM campaign_emails ce
		WHERE ce.campaign_id = $1
	`
	args := []interface{}{id}

	// Handle special filters for opened/clicked
	if status == "opened" {
		query += " AND ce.opened_at IS NOT NULL"
	} else if status == "clicked" {
		query += " AND ce.clicked_at IS NOT NULL"
	} else if status != "" {
		query += " AND ce.status = $2"
		args = append(args, status)
	}

	query += " ORDER BY ce.sent_at DESC NULLS LAST LIMIT $" + fmt.Sprintf("%d", len(args)+1) + " OFFSET $" + fmt.Sprintf("%d", len(args)+2)
	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		log.Printf("[Campaign] Error fetching details: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch details"})
	}
	defer rows.Close()

	var emails []fiber.Map
	for rows.Next() {
		var emailStatus string
		var email, name, errorMsg *string
		var sentAt, openedAt, clickedAt *time.Time

		rows.Scan(&email, &name, &emailStatus, &sentAt, &openedAt, &clickedAt, &errorMsg)

		emailStr := ""
		if email != nil {
			emailStr = *email
		}

		emails = append(emails, fiber.Map{
			"email":      emailStr,
			"name":       name,
			"status":     emailStatus,
			"sent_at":    sentAt,
			"opened_at":  openedAt,
			"clicked_at": clickedAt,
			"error":      errorMsg,
		})
	}

	// Get total count
	var total int
	countQuery := `SELECT COUNT(*) FROM campaign_emails WHERE campaign_id = $1`
	countArgs := []interface{}{id}
	if status == "opened" {
		countQuery += " AND opened_at IS NOT NULL"
	} else if status == "clicked" {
		countQuery += " AND clicked_at IS NOT NULL"
	} else if status != "" {
		countQuery += " AND status = $2"
		countArgs = append(countArgs, status)
	}
	s.db.QueryRow(countQuery, countArgs...).Scan(&total)

	return c.JSON(fiber.Map{
		"data":  emails,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// exportCampaignEmails exports emails that opened or clicked (only email addresses, one per line)
func (s *Server) exportCampaignEmails(c *fiber.Ctx) error {
	userID := getUserID(c)
	id := c.Params("id")
	exportType := c.Query("type", "opened") // opened, clicked

	// Verify campaign exists and belongs to user
	var campaignName string
	err := s.db.QueryRow(`SELECT name FROM campaigns WHERE id = $1 AND user_id = $2`, id, userID).Scan(&campaignName)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Campaign not found"})
	}

	// Build query based on export type - only select email
	var query string
	// Use stored email directly - works with both ClickHouse and PostgreSQL emails
	switch exportType {
	case "opened":
		query = `
			SELECT ce.email
			FROM campaign_emails ce
			WHERE ce.campaign_id = $1 AND ce.opened_at IS NOT NULL AND ce.email IS NOT NULL
			ORDER BY ce.opened_at DESC
		`
	case "clicked":
		query = `
			SELECT ce.email
			FROM campaign_emails ce
			WHERE ce.campaign_id = $1 AND ce.clicked_at IS NOT NULL AND ce.email IS NOT NULL
			ORDER BY ce.clicked_at DESC
		`
	default:
		query = `
			SELECT ce.email
			FROM campaign_emails ce
			WHERE ce.campaign_id = $1 AND ce.email IS NOT NULL
			ORDER BY ce.created_at DESC
		`
	}

	rows, err := s.db.Query(query, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch emails"})
	}
	defer rows.Close()

	// Build simple list - one email per line
	var result strings.Builder
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			continue
		}
		result.WriteString(email + "\n")
	}

	// Set headers for file download
	filename := fmt.Sprintf("%s_%s.txt", strings.ReplaceAll(campaignName, " ", "_"), exportType)
	c.Set("Content-Type", "text/plain; charset=utf-8")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

	return c.SendString(result.String())
}
