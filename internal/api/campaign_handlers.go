package api

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"smtpenviador/internal/queue"
)

// getTrackingDomain returns the tracking domain from settings
func (s *Server) getTrackingDomain() string {
	var domain string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = 'tracking_domain'`).Scan(&domain)
	if err != nil {
		return s.cfg.TrackingDomain // Fallback to config
	}
	return domain
}

type CampaignRequest struct {
	Name        string     `json:"name"`
	Subject     string     `json:"subject"`
	FromName    string     `json:"from_name"`
	FromEmail   string     `json:"from_email"`
	ReplyTo     string     `json:"reply_to"`
	HTMLContent string     `json:"html_content"`
	TextContent string     `json:"text_content"`
	ListID      string     `json:"list_id"`
	SendRate    int        `json:"send_rate"`
	ScheduledAt *time.Time `json:"scheduled_at"`
	TrackOpens  bool       `json:"track_opens"`
	TrackClicks bool       `json:"track_clicks"`
}

// listCampaigns returns all campaigns
func (s *Server) listCampaigns(c *fiber.Ctx) error {
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
	`
	args := []interface{}{}

	if status != "" {
		query += " WHERE c.status = $1"
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
			"id":             id,
			"name":           name,
			"subject":        subject,
			"from_name":      fromName,
			"from_email":     fromEmail,
			"status":         campaignStatus,
			"total_emails":   totalEmails,
			"sent_count":     sentCount,
			"failed_count":   failedCount,
			"open_count":     openCount,
			"click_count":    clickCount,
			"bounce_count":   bounceCount,
			"scheduled_at":   scheduledAt,
			"auto_start_at":  autoStartAtUnix,
			"started_at":     startedAt,
			"completed_at":   completedAt,
			"created_at":     createdAt,
			"list_name":      listName,
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
	var req CampaignRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// Validate - from_email not required anymore (comes from SMTP senders)
	if req.Name == "" || req.Subject == "" || req.HTMLContent == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Missing required fields"})
	}

	if req.ListID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "List ID is required"})
	}

	// Check if there are active SMTPs available
	var smtpCount int
	s.db.QueryRow(`SELECT COUNT(*) FROM smtp_servers WHERE active = true`).Scan(&smtpCount)

	if smtpCount == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Nenhum SMTP ativo disponível. Adicione um SMTP antes de criar campanhas."})
	}

	// Get email count from list
	var totalEmails int
	s.db.QueryRow(`SELECT COUNT(*) FROM emails WHERE list_id = $1 AND valid = true AND bounced = false AND unsubscribed = false`, req.ListID).Scan(&totalEmails)

	id := uuid.New().String()

	// Insert campaign as draft with auto_start_at = NOW() + 60 seconds (in UTC)
	autoStartAt := time.Now().UTC().Add(60 * time.Second)
	_, err := s.db.Exec(`
		INSERT INTO campaigns (id, name, subject, from_name, from_email, reply_to,
		                       html_content, text_content, list_id, send_rate,
		                       scheduled_at, track_opens, track_clicks, total_emails, status, auto_start_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, 'draft', $15)
	`, id, req.Name, req.Subject, req.FromName, req.FromEmail, req.ReplyTo,
		req.HTMLContent, req.TextContent, req.ListID, req.SendRate,
		req.ScheduledAt, req.TrackOpens, req.TrackClicks, totalEmails, autoStartAt)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create campaign"})
	}

	return c.Status(201).JSON(fiber.Map{
		"message":       "Campanha criada",
		"id":            id,
		"total_emails":  totalEmails,
		"smtp_count":    smtpCount,
		"auto_start_at": autoStartAt.Unix(),
	})
}

// getCampaign returns a single campaign
func (s *Server) getCampaign(c *fiber.Ctx) error {
	id := c.Params("id")

	var name, subject, fromName, fromEmail, replyTo, htmlContent, textContent, status, listID string
	var totalEmails, sentCount, failedCount, openCount, clickCount, bounceCount, sendRate int
	var scheduledAt, startedAt, completedAt *time.Time
	var createdAt, updatedAt time.Time

	err := s.db.QueryRow(`
		SELECT name, subject, from_name, from_email, reply_to, html_content, text_content,
		       list_id, status, total_emails, sent_count, failed_count, open_count,
		       click_count, bounce_count, send_rate, scheduled_at, started_at,
		       completed_at, created_at, updated_at
		FROM campaigns WHERE id = $1
	`, id).Scan(&name, &subject, &fromName, &fromEmail, &replyTo, &htmlContent, &textContent,
		&listID, &status, &totalEmails, &sentCount, &failedCount, &openCount,
		&clickCount, &bounceCount, &sendRate, &scheduledAt, &startedAt,
		&completedAt, &createdAt, &updatedAt)

	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Campaign not found"})
	}

	return c.JSON(fiber.Map{
		"id":            id,
		"name":          name,
		"subject":       subject,
		"from_name":     fromName,
		"from_email":    fromEmail,
		"reply_to":      replyTo,
		"html_content":  htmlContent,
		"text_content":  textContent,
		"list_id":       listID,
		"status":        status,
		"total_emails":  totalEmails,
		"sent_count":    sentCount,
		"failed_count":  failedCount,
		"open_count":    openCount,
		"click_count":   clickCount,
		"bounce_count":  bounceCount,
		"send_rate":     sendRate,
		"scheduled_at":  scheduledAt,
		"started_at":    startedAt,
		"completed_at":  completedAt,
		"created_at":    createdAt,
		"updated_at":    updatedAt,
	})
}

// updateCampaign updates a campaign
func (s *Server) updateCampaign(c *fiber.Ctx) error {
	id := c.Params("id")

	var req CampaignRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// Check if campaign is editable
	var status string
	s.db.QueryRow(`SELECT status FROM campaigns WHERE id = $1`, id).Scan(&status)
	if status != "draft" && status != "paused" {
		return c.Status(400).JSON(fiber.Map{"error": "Campaign cannot be edited in current status"})
	}

	result, err := s.db.Exec(`
		UPDATE campaigns SET
			name = $1, subject = $2, from_name = $3, from_email = $4,
			reply_to = $5, html_content = $6, text_content = $7,
			list_id = $8, send_rate = $9, scheduled_at = $10
		WHERE id = $11
	`, req.Name, req.Subject, req.FromName, req.FromEmail, req.ReplyTo,
		req.HTMLContent, req.TextContent, req.ListID, req.SendRate,
		req.ScheduledAt, id)

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
	id := c.Params("id")

	result, err := s.db.Exec(`DELETE FROM campaigns WHERE id = $1`, id)
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
	id := c.Params("id")

	// Get campaign details
	var listID, fromEmail, fromName, replyTo, subject, htmlContent, textContent string
	var status string
	var trackOpens, trackClicks bool
	err := s.db.QueryRow(`
		SELECT list_id, from_email, from_name, reply_to, subject, html_content, text_content, status, COALESCE(track_opens, true), COALESCE(track_clicks, true)
		FROM campaigns WHERE id = $1
	`, id).Scan(&listID, &fromEmail, &fromName, &replyTo, &subject, &htmlContent, &textContent, &status, &trackOpens, &trackClicks)

	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Campaign not found"})
	}

	if status != "draft" && status != "paused" {
		return c.Status(400).JSON(fiber.Map{"error": "Campaign cannot be started in current status"})
	}

	// Get tracking domain from settings
	trackingDomain := s.getTrackingDomain()

	// Get emails from list
	rows, err := s.db.Query(`
		SELECT id, email, name, custom1, custom2, custom3, custom4, custom5
		FROM emails
		WHERE list_id = $1 AND valid = true AND bounced = false AND unsubscribed = false
	`, listID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch emails"})
	}
	defer rows.Close()

	// Queue emails
	count := 0
	for rows.Next() {
		var emailID, email string
		var name, c1, c2, c3, c4, c5 *string
		rows.Scan(&emailID, &email, &name, &c1, &c2, &c3, &c4, &c5)

		// Create variables map
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
			CampaignID:     id,
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

		// Insert campaign_email record
		_, err = s.db.Exec(`
			INSERT INTO campaign_emails (id, campaign_id, email_id, status)
			VALUES ($1, $2, $3, 'queued')
		`, job.ID, id, emailID)
		if err != nil {
			log.Printf("❌ Failed to insert campaign_email for %s: %v", email, err)
			continue // Skip this email if we can't track it
		}

		// Push to queue
		if err := s.queue.Push(job); err != nil {
			log.Printf("❌ Failed to push job to queue for %s: %v", email, err)
			continue
		}
		count++
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
	id := c.Params("id")

	result, err := s.db.Exec(`
		UPDATE campaigns SET status = 'paused' WHERE id = $1 AND status = 'running'
	`, id)

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
	id := c.Params("id")

	result, err := s.db.Exec(`
		UPDATE campaigns SET status = 'running' WHERE id = $1 AND status = 'paused'
	`, id)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to resume campaign"})
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Campaign is not paused"})
	}

	return c.JSON(fiber.Map{"message": "Campaign resumed"})
}

// cancelCampaign cancels a campaign
func (s *Server) cancelCampaign(c *fiber.Ctx) error {
	id := c.Params("id")

	result, err := s.db.Exec(`
		UPDATE campaigns SET status = 'cancelled' WHERE id = $1 AND status IN ('running', 'paused', 'scheduled')
	`, id)

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
	id := c.Params("id")

	result, err := s.db.Exec(`
		UPDATE campaigns SET auto_start_at = NULL WHERE id = $1 AND status = 'draft'
	`, id)

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
	result, err := s.db.Exec(`
		UPDATE campaigns
		SET scheduled_at = $1, status = 'scheduled', auto_start_at = NULL
		WHERE id = $2 AND status = 'draft'
	`, scheduledAt, id)
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
	id := c.Params("id")

	result, err := s.db.Exec(`
		UPDATE campaigns
		SET scheduled_at = NULL, status = 'draft'
		WHERE id = $1 AND status = 'scheduled'
	`, id)
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

	// Get campaign details
	var listID, fromEmail, fromName, replyTo, subject, htmlContent, textContent string
	var trackOpens, trackClicks bool
	err := s.db.QueryRow(`
		SELECT list_id, from_email, from_name, reply_to, subject, html_content, text_content, COALESCE(track_opens, true), COALESCE(track_clicks, true)
		FROM campaigns WHERE id = $1 AND status = 'draft'
	`, id).Scan(&listID, &fromEmail, &fromName, &replyTo, &subject, &htmlContent, &textContent, &trackOpens, &trackClicks)

	if err != nil {
		fmt.Printf("[AutoStart] Error getting campaign %s: %v\n", id, err)
		return
	}

	fmt.Printf("[AutoStart] Campaign %s - List: %s, From: %s\n", id, listID, fromEmail)

	// Get tracking domain from settings
	trackingDomain := s.getTrackingDomain()

	// Get emails from list
	rows, err := s.db.Query(`
		SELECT id, email, name, custom1, custom2, custom3, custom4, custom5
		FROM emails
		WHERE list_id = $1 AND valid = true AND bounced = false AND unsubscribed = false
	`, listID)
	if err != nil {
		fmt.Printf("[AutoStart] Error getting emails for campaign %s: %v\n", id, err)
		return
	}
	defer rows.Close()

	// Queue emails
	count := s.queueEmails(rows, id, fromEmail, fromName, replyTo, subject, htmlContent, textContent, trackOpens, trackClicks, trackingDomain)
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
	id := c.Params("id")

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
	id := c.Params("id")

	// Check if there are active SMTPs available
	var smtpCount int
	s.db.QueryRow(`SELECT COUNT(*) FROM smtp_servers WHERE active = true`).Scan(&smtpCount)

	if smtpCount == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Nenhum SMTP ativo disponível. Adicione um SMTP antes de clonar campanhas."})
	}

	// Get original campaign
	var name, subject, fromName, fromEmail, replyTo, htmlContent, textContent, listID string
	var sendRate int
	var trackOpens, trackClicks bool
	err := s.db.QueryRow(`
		SELECT name, subject, from_name, from_email, reply_to, html_content, text_content, list_id, send_rate, COALESCE(track_opens, true), COALESCE(track_clicks, true)
		FROM campaigns WHERE id = $1
	`, id).Scan(&name, &subject, &fromName, &fromEmail, &replyTo, &htmlContent, &textContent, &listID, &sendRate, &trackOpens, &trackClicks)

	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Campaign not found"})
	}

	// Get email count from list
	var totalEmails int
	s.db.QueryRow(`SELECT COUNT(*) FROM emails WHERE list_id = $1 AND valid = true AND bounced = false AND unsubscribed = false`, listID).Scan(&totalEmails)

	// Create new campaign with "Copy of" prefix as draft with auto_start_at = NOW() + 60 seconds (UTC)
	newID := uuid.New().String()
	newName := "Cópia de " + name
	autoStartAt := time.Now().UTC().Add(60 * time.Second)

	_, err = s.db.Exec(`
		INSERT INTO campaigns (id, name, subject, from_name, from_email, reply_to, html_content, text_content, list_id, send_rate, track_opens, track_clicks, total_emails, status, auto_start_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, 'draft', $14)
	`, newID, newName, subject, fromName, fromEmail, replyTo, htmlContent, textContent, listID, sendRate, trackOpens, trackClicks, totalEmails, autoStartAt)

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
	id := c.Params("id")

	// Get campaign details
	var listID, fromEmail, fromName, replyTo, subject, htmlContent, textContent string
	var trackOpens, trackClicks bool
	err := s.db.QueryRow(`
		SELECT list_id, from_email, from_name, reply_to, subject, html_content, text_content, COALESCE(track_opens, true), COALESCE(track_clicks, true)
		FROM campaigns WHERE id = $1
	`, id).Scan(&listID, &fromEmail, &fromName, &replyTo, &subject, &htmlContent, &textContent, &trackOpens, &trackClicks)

	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Campaign not found"})
	}

	// Get tracking domain from settings
	trackingDomain := s.getTrackingDomain()

	// Clear previous campaign_emails
	s.db.Exec(`DELETE FROM campaign_emails WHERE campaign_id = $1`, id)

	// Reset campaign counters
	s.db.Exec(`UPDATE campaigns SET sent_count = 0, failed_count = 0, open_count = 0, click_count = 0, bounce_count = 0 WHERE id = $1`, id)

	// Get all emails from list
	rows, err := s.db.Query(`
		SELECT id, email, name, custom1, custom2, custom3, custom4, custom5
		FROM emails
		WHERE list_id = $1 AND valid = true AND bounced = false AND unsubscribed = false
	`, listID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch emails"})
	}
	defer rows.Close()

	count := s.queueEmails(rows, id, fromEmail, fromName, replyTo, subject, htmlContent, textContent, trackOpens, trackClicks, trackingDomain)

	// Update campaign status
	s.db.Exec(`UPDATE campaigns SET status = 'running', started_at = NOW(), total_emails = $1 WHERE id = $2`, count, id)

	return c.JSON(fiber.Map{
		"message":       "Campaign resend started",
		"emails_queued": count,
	})
}

// resendToFailed resends only to emails that failed
func (s *Server) resendToFailed(c *fiber.Ctx) error {
	id := c.Params("id")

	// Get campaign details
	var fromEmail, fromName, replyTo, subject, htmlContent, textContent string
	var trackOpens, trackClicks bool
	err := s.db.QueryRow(`
		SELECT from_email, from_name, reply_to, subject, html_content, text_content, COALESCE(track_opens, true), COALESCE(track_clicks, true)
		FROM campaigns WHERE id = $1
	`, id).Scan(&fromEmail, &fromName, &replyTo, &subject, &htmlContent, &textContent, &trackOpens, &trackClicks)

	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Campaign not found"})
	}

	// Get tracking domain from settings
	trackingDomain := s.getTrackingDomain()

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
	id := c.Params("id")

	// Get campaign details
	var fromEmail, fromName, replyTo, subject, htmlContent, textContent string
	var trackOpens, trackClicks bool
	err := s.db.QueryRow(`
		SELECT from_email, from_name, reply_to, subject, html_content, text_content, COALESCE(track_opens, true), COALESCE(track_clicks, true)
		FROM campaigns WHERE id = $1
	`, id).Scan(&fromEmail, &fromName, &replyTo, &subject, &htmlContent, &textContent, &trackOpens, &trackClicks)

	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Campaign not found"})
	}

	// Get tracking domain from settings
	trackingDomain := s.getTrackingDomain()

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
func (s *Server) queueEmails(rows *sql.Rows, campaignID, fromEmail, fromName, replyTo, subject, htmlContent, textContent string, trackOpens, trackClicks bool, trackingDomain string) int {
	count := 0
	var err error
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

		_, err = s.db.Exec(`
			INSERT INTO campaign_emails (id, campaign_id, email_id, status)
			VALUES ($1, $2, $3, 'queued')
		`, job.ID, campaignID, emailID)
		if err != nil {
			log.Printf("❌ Failed to insert campaign_email for %s: %v", email, err)
			continue
		}

		if err := s.queue.Push(job); err != nil {
			log.Printf("❌ Failed to push job to queue for %s: %v", email, err)
			continue
		}
		count++
	}
	return count
}

// exportCampaignCSV exports campaign results to CSV
func (s *Server) exportCampaignCSV(c *fiber.Ctx) error {
	id := c.Params("id")

	// Get campaign name
	var campaignName string
	s.db.QueryRow(`SELECT name FROM campaigns WHERE id = $1`, id).Scan(&campaignName)

	// Get all campaign emails with details
	rows, err := s.db.Query(`
		SELECT e.email, e.name, ce.status, ce.sent_at, ce.opened_at, ce.clicked_at, ce.error_message
		FROM campaign_emails ce
		INNER JOIN emails e ON e.id = ce.email_id
		WHERE ce.campaign_id = $1
		ORDER BY ce.sent_at DESC
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
	id := c.Params("id")
	status := c.Query("status", "")
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 50)
	offset := (page - 1) * limit

	// Build query
	query := `
		SELECT e.email, e.name, ce.status, ce.sent_at, ce.opened_at, ce.clicked_at, ce.error_message
		FROM campaign_emails ce
		INNER JOIN emails e ON e.id = ce.email_id
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

	query += " ORDER BY ce.sent_at DESC LIMIT $" + fmt.Sprintf("%d", len(args)+1) + " OFFSET $" + fmt.Sprintf("%d", len(args)+2)
	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch details"})
	}
	defer rows.Close()

	var emails []fiber.Map
	for rows.Next() {
		var email, emailStatus string
		var name, errorMsg *string
		var sentAt, openedAt, clickedAt *time.Time

		rows.Scan(&email, &name, &emailStatus, &sentAt, &openedAt, &clickedAt, &errorMsg)

		emails = append(emails, fiber.Map{
			"email":      email,
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
	id := c.Params("id")
	exportType := c.Query("type", "opened") // opened, clicked

	// Verify campaign exists
	var campaignName string
	err := s.db.QueryRow(`SELECT name FROM campaigns WHERE id = $1`, id).Scan(&campaignName)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Campaign not found"})
	}

	// Build query based on export type - only select email
	var query string
	switch exportType {
	case "opened":
		query = `
			SELECT e.email
			FROM campaign_emails ce
			JOIN emails e ON ce.email_id = e.id
			WHERE ce.campaign_id = $1 AND ce.opened_at IS NOT NULL
			ORDER BY ce.opened_at DESC
		`
	case "clicked":
		query = `
			SELECT e.email
			FROM campaign_emails ce
			JOIN emails e ON ce.email_id = e.id
			WHERE ce.campaign_id = $1 AND ce.clicked_at IS NOT NULL
			ORDER BY ce.clicked_at DESC
		`
	default:
		query = `
			SELECT e.email
			FROM campaign_emails ce
			JOIN emails e ON ce.email_id = e.id
			WHERE ce.campaign_id = $1
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
