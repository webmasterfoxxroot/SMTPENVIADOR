package api

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"smtpenviador/internal/queue"
)

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
}

// listCampaigns returns all campaigns
func (s *Server) listCampaigns(c *fiber.Ctx) error {
	status := c.Query("status", "")

	query := `
		SELECT c.id, c.name, c.subject, c.from_name, c.from_email, c.status,
		       c.total_emails, c.sent_count, c.failed_count, c.open_count,
		       c.click_count, c.bounce_count, c.scheduled_at, c.started_at,
		       c.completed_at, c.created_at, l.name as list_name
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
		var createdAt time.Time

		err := rows.Scan(&id, &name, &subject, &fromName, &fromEmail, &campaignStatus,
			&totalEmails, &sentCount, &failedCount, &openCount, &clickCount, &bounceCount,
			&scheduledAt, &startedAt, &completedAt, &createdAt, &listName)
		if err != nil {
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
			"started_at":    startedAt,
			"completed_at":  completedAt,
			"created_at":    createdAt,
			"list_name":     listName,
		})
	}

	return c.JSON(fiber.Map{
		"data":  campaigns,
		"total": len(campaigns),
	})
}

// createCampaign creates a new campaign
func (s *Server) createCampaign(c *fiber.Ctx) error {
	var req CampaignRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// Validate
	if req.Name == "" || req.Subject == "" || req.FromEmail == "" || req.HTMLContent == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Missing required fields"})
	}

	id := uuid.New().String()

	// Get email count from list
	var totalEmails int
	if req.ListID != "" {
		s.db.QueryRow(`SELECT valid_emails FROM email_lists WHERE id = $1`, req.ListID).Scan(&totalEmails)
	}

	_, err := s.db.Exec(`
		INSERT INTO campaigns (id, name, subject, from_name, from_email, reply_to,
		                       html_content, text_content, list_id, send_rate,
		                       scheduled_at, total_emails, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, 'draft')
	`, id, req.Name, req.Subject, req.FromName, req.FromEmail, req.ReplyTo,
		req.HTMLContent, req.TextContent, req.ListID, req.SendRate,
		req.ScheduledAt, totalEmails)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create campaign"})
	}

	return c.Status(201).JSON(fiber.Map{
		"message": "Campaign created",
		"id":      id,
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
	err := s.db.QueryRow(`
		SELECT list_id, from_email, from_name, reply_to, subject, html_content, text_content, status
		FROM campaigns WHERE id = $1
	`, id).Scan(&listID, &fromEmail, &fromName, &replyTo, &subject, &htmlContent, &textContent, &status)

	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Campaign not found"})
	}

	if status != "draft" && status != "paused" {
		return c.Status(400).JSON(fiber.Map{"error": "Campaign cannot be started in current status"})
	}

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
			ID:          uuid.New().String(),
			CampaignID:  id,
			EmailID:     emailID,
			To:          email,
			ToName:      nameStr,
			From:        fromEmail,
			FromName:    fromName,
			ReplyTo:     replyTo,
			Subject:     subject,
			HTMLContent: htmlContent,
			TextContent: textContent,
			Variables:   variables,
			CreatedAt:   time.Now(),
		}

		// Insert campaign_email record
		s.db.Exec(`
			INSERT INTO campaign_emails (id, campaign_id, email_id, status)
			VALUES ($1, $2, $3, 'queued')
		`, job.ID, id, emailID)

		// Push to queue
		s.queue.Push(job)
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
