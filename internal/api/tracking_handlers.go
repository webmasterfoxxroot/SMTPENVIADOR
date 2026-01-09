package api

import (
	"context"
	"log"
	"net/url"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// 1x1 transparent GIF
var trackingPixel = []byte{
	0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00,
	0x01, 0x00, 0x80, 0x00, 0x00, 0xff, 0xff, 0xff,
	0x00, 0x00, 0x00, 0x21, 0xf9, 0x04, 0x01, 0x00,
	0x00, 0x00, 0x00, 0x2c, 0x00, 0x00, 0x00, 0x00,
	0x01, 0x00, 0x01, 0x00, 0x00, 0x02, 0x02, 0x44,
	0x01, 0x00, 0x3b,
}

// trackOpen handles email open tracking
func (s *Server) trackOpen(c *fiber.Ctx) error {
	campaignID := c.Params("campaignId")
	emailID := c.Params("emailId")

	log.Printf("[Tracking] Open request: campaign=%s, email=%s, IP=%s", campaignID, emailID, c.IP())

	// Check if this is the first open for this email
	var alreadyOpened bool
	var exists bool
	err := s.db.QueryRow(`
		SELECT opened_at IS NOT NULL FROM campaign_emails
		WHERE campaign_id = $1 AND email_id = $2
	`, campaignID, emailID).Scan(&alreadyOpened)

	if err != nil {
		log.Printf("[Tracking] No campaign_emails record found for campaign=%s, email=%s: %v", campaignID, emailID, err)
		// Try to find by campaign_emails.id instead (maybe emailID is actually the campaign_email ID)
		err = s.db.QueryRow(`
			SELECT opened_at IS NOT NULL FROM campaign_emails
			WHERE campaign_id = $1 AND id = $2
		`, campaignID, emailID).Scan(&alreadyOpened)
		if err != nil {
			log.Printf("[Tracking] Also no record by id: %v", err)
		} else {
			exists = true
			log.Printf("[Tracking] Found record by campaign_emails.id")
		}
	} else {
		exists = true
	}

	// Record open event (for analytics)
	go s.recordEvent(campaignID, emailID, "open", "", c.IP(), c.Get("User-Agent"))

	// Only count if first open and record exists
	if exists && !alreadyOpened {
		log.Printf("[Tracking] Recording first open for campaign=%s, email=%s", campaignID, emailID)

		// Update campaign_emails - also set status to 'sent' if still queued (tracking proves delivery)
		result, err := s.db.Exec(`
			UPDATE campaign_emails
			SET opened_at = NOW(),
			    status = CASE WHEN status = 'queued' THEN 'sent' ELSE status END,
			    sent_at = CASE WHEN sent_at IS NULL THEN NOW() ELSE sent_at END
			WHERE campaign_id = $1 AND email_id = $2 AND opened_at IS NULL
		`, campaignID, emailID)

		if err != nil {
			log.Printf("[Tracking] Failed to update campaign_emails: %v", err)
		} else {
			rowsAffected, _ := result.RowsAffected()
			log.Printf("[Tracking] Updated %d rows in campaign_emails", rowsAffected)
		}

		// Update campaign open count (only once per email)
		s.db.Exec(`
			UPDATE campaigns SET open_count = open_count + 1 WHERE id = $1
		`, campaignID)

		// Also update sent_count if status was queued
		s.db.Exec(`
			UPDATE campaigns SET sent_count = sent_count + 1
			WHERE id = $1 AND EXISTS (
				SELECT 1 FROM campaign_emails
				WHERE campaign_id = $1 AND email_id = $2 AND status = 'sent'
			)
		`, campaignID, emailID)

		// Increment Redis stat
		go s.queue.IncrementStat("opened", 1)
	} else if alreadyOpened {
		log.Printf("[Tracking] Email already opened, not counting again")
	}

	// Return tracking pixel
	c.Set("Content-Type", "image/gif")
	c.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	return c.Send(trackingPixel)
}

// trackClick handles link click tracking
func (s *Server) trackClick(c *fiber.Ctx) error {
	campaignID := c.Params("campaignId")
	emailID := c.Params("emailId")
	targetURL := c.Query("url")

	log.Printf("[Tracking] Click request: campaign=%s, email=%s, url=%s, IP=%s", campaignID, emailID, targetURL, c.IP())

	if targetURL == "" {
		return c.Status(400).SendString("Missing URL")
	}

	// Decode URL
	decodedURL, err := url.QueryUnescape(targetURL)
	if err != nil {
		decodedURL = targetURL
	}

	// Check if this is the first click for this email
	var alreadyClicked bool
	var exists bool
	err = s.db.QueryRow(`
		SELECT clicked_at IS NOT NULL FROM campaign_emails
		WHERE campaign_id = $1 AND email_id = $2
	`, campaignID, emailID).Scan(&alreadyClicked)

	if err != nil {
		log.Printf("[Tracking] No campaign_emails record found for click: campaign=%s, email=%s: %v", campaignID, emailID, err)
		// Try to find by campaign_emails.id instead
		err = s.db.QueryRow(`
			SELECT clicked_at IS NOT NULL FROM campaign_emails
			WHERE campaign_id = $1 AND id = $2
		`, campaignID, emailID).Scan(&alreadyClicked)
		if err != nil {
			log.Printf("[Tracking] Also no record by id: %v", err)
		} else {
			exists = true
		}
	} else {
		exists = true
	}

	// Record click event (for analytics - all clicks)
	go s.recordEvent(campaignID, emailID, "click", decodedURL, c.IP(), c.Get("User-Agent"))

	// Only count if first click and record exists
	if exists && !alreadyClicked {
		log.Printf("[Tracking] Recording first click for campaign=%s, email=%s", campaignID, emailID)

		// Update campaign_emails - also set status to 'sent' if still queued (tracking proves delivery)
		result, err := s.db.Exec(`
			UPDATE campaign_emails
			SET clicked_at = NOW(),
			    status = CASE WHEN status = 'queued' THEN 'sent' ELSE status END,
			    sent_at = CASE WHEN sent_at IS NULL THEN NOW() ELSE sent_at END
			WHERE campaign_id = $1 AND email_id = $2 AND clicked_at IS NULL
		`, campaignID, emailID)

		if err != nil {
			log.Printf("[Tracking] Failed to update campaign_emails for click: %v", err)
		} else {
			rowsAffected, _ := result.RowsAffected()
			log.Printf("[Tracking] Updated %d rows for click", rowsAffected)
		}

		// Update campaign click count (only once per email)
		s.db.Exec(`
			UPDATE campaigns SET click_count = click_count + 1 WHERE id = $1
		`, campaignID)

		// Increment Redis stat
		go s.queue.IncrementStat("clicked", 1)
	} else if alreadyClicked {
		log.Printf("[Tracking] Email already clicked, not counting again")
	}

	// Redirect to target URL
	return c.Redirect(decodedURL, 302)
}

// unsubscribe handles unsubscribe requests
func (s *Server) unsubscribe(c *fiber.Ctx) error {
	campaignID := c.Params("campaignId")
	emailID := c.Params("emailId")

	// Get email address - try ClickHouse first, then PostgreSQL
	var email string
	var found bool

	if s.ch != nil {
		ctx := context.Background()
		chEmail, err := s.ch.GetEmailByID(ctx, emailID)
		if err == nil && chEmail != "" {
			email = chEmail
			found = true
			// Mark as unsubscribed in ClickHouse
			s.ch.MarkEmailUnsubscribed(ctx, emailID)
		}
	}

	// Fallback to PostgreSQL
	if !found {
		err := s.db.QueryRow(`SELECT email FROM emails WHERE id = $1`, emailID).Scan(&email)
		if err != nil {
			return c.Status(404).SendString("Email not found")
		}
		// Mark as unsubscribed in PostgreSQL
		s.db.Exec(`UPDATE emails SET unsubscribed = true WHERE id = $1`, emailID)
	}

	// Add to blacklist (PostgreSQL for quick lookups, also to ClickHouse if available)
	s.db.Exec(`
		INSERT INTO blacklist (id, email, reason)
		VALUES ($1, $2, 'unsubscribe')
		ON CONFLICT (email) DO NOTHING
	`, uuid.New().String(), email)

	// Also add to ClickHouse blacklist if available
	if s.ch != nil {
		ctx := context.Background()
		s.ch.InsertBlacklistBatch(ctx, []string{email}, "unsubscribe")
	}

	// Record event
	go s.recordEvent(campaignID, emailID, "unsubscribe", "", c.IP(), c.Get("User-Agent"))

	// Update campaign unsubscribe count
	go s.db.Exec(`
		UPDATE campaigns SET unsubscribe_count = unsubscribe_count + 1 WHERE id = $1
	`, campaignID)

	// Return unsubscribe page
	return c.SendString(`
		<!DOCTYPE html>
		<html>
		<head>
			<title>Descadastrado</title>
			<style>
				body { font-family: Arial, sans-serif; text-align: center; padding: 50px; }
				h1 { color: #333; }
				p { color: #666; }
			</style>
		</head>
		<body>
			<h1>Descadastrado com Sucesso</h1>
			<p>Você foi removido da nossa lista de emails.</p>
			<p>Você não receberá mais emails nossos.</p>
		</body>
		</html>
	`)
}

// recordEvent saves a tracking event
func (s *Server) recordEvent(campaignID, emailID, eventType, linkURL, ip, userAgent string) {
	s.db.Exec(`
		INSERT INTO tracking_events (id, campaign_id, email_id, event_type, link_url, ip_address, user_agent)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, uuid.New().String(), campaignID, emailID, eventType, linkURL, ip, userAgent)
}
