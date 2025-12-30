package api

import (
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

	// Check if this is the first open for this email
	var alreadyOpened bool
	s.db.QueryRow(`
		SELECT opened_at IS NOT NULL FROM campaign_emails
		WHERE campaign_id = $1 AND email_id = $2
	`, campaignID, emailID).Scan(&alreadyOpened)

	// Record open event (for analytics)
	go s.recordEvent(campaignID, emailID, "open", "", c.IP(), c.Get("User-Agent"))

	// Only count if first open
	if !alreadyOpened {
		// Update campaign_emails
		s.db.Exec(`
			UPDATE campaign_emails SET opened_at = NOW()
			WHERE campaign_id = $1 AND email_id = $2 AND opened_at IS NULL
		`, campaignID, emailID)

		// Update campaign open count (only once per email)
		s.db.Exec(`
			UPDATE campaigns SET open_count = open_count + 1 WHERE id = $1
		`, campaignID)

		// Increment Redis stat
		go s.queue.IncrementStat("opened", 1)
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
	s.db.QueryRow(`
		SELECT clicked_at IS NOT NULL FROM campaign_emails
		WHERE campaign_id = $1 AND email_id = $2
	`, campaignID, emailID).Scan(&alreadyClicked)

	// Record click event (for analytics - all clicks)
	go s.recordEvent(campaignID, emailID, "click", decodedURL, c.IP(), c.Get("User-Agent"))

	// Only count if first click
	if !alreadyClicked {
		// Update campaign_emails
		s.db.Exec(`
			UPDATE campaign_emails SET clicked_at = NOW()
			WHERE campaign_id = $1 AND email_id = $2 AND clicked_at IS NULL
		`, campaignID, emailID)

		// Update campaign click count (only once per email)
		s.db.Exec(`
			UPDATE campaigns SET click_count = click_count + 1 WHERE id = $1
		`, campaignID)

		// Increment Redis stat
		go s.queue.IncrementStat("clicked", 1)
	}

	// Redirect to target URL
	return c.Redirect(decodedURL, 302)
}

// unsubscribe handles unsubscribe requests
func (s *Server) unsubscribe(c *fiber.Ctx) error {
	campaignID := c.Params("campaignId")
	emailID := c.Params("emailId")

	// Get email address
	var email string
	err := s.db.QueryRow(`SELECT email FROM emails WHERE id = $1`, emailID).Scan(&email)
	if err != nil {
		return c.Status(404).SendString("Email not found")
	}

	// Mark as unsubscribed
	s.db.Exec(`UPDATE emails SET unsubscribed = true WHERE id = $1`, emailID)

	// Add to blacklist
	s.db.Exec(`
		INSERT INTO blacklist (id, email, reason)
		VALUES ($1, $2, 'unsubscribe')
		ON CONFLICT (email) DO NOTHING
	`, uuid.New().String(), email)

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
			<title>Unsubscribed</title>
			<style>
				body { font-family: Arial, sans-serif; text-align: center; padding: 50px; }
				h1 { color: #333; }
				p { color: #666; }
			</style>
		</head>
		<body>
			<h1>Unsubscribed Successfully</h1>
			<p>You have been removed from our mailing list.</p>
			<p>You will no longer receive emails from us.</p>
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
