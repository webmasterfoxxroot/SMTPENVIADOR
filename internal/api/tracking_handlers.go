package api

import (
	"context"
	"database/sql"
	"log"
	"net/url"
	"sync"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"smtpenviador/internal/tracking"
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

// Global tracking service (singleton)
var (
	trackingSvc     *tracking.TrackingService
	trackingSvcOnce sync.Once
)

// getTrackingService returns the singleton tracking service
func getTrackingService() *tracking.TrackingService {
	trackingSvcOnce.Do(func() {
		trackingSvc = tracking.NewTrackingService()
		log.Println("[Tracking] Tracking service initialized with geolocation and anti-bot detection")
	})
	return trackingSvc
}

// trackOpen handles email open tracking
func (s *Server) trackOpen(c *fiber.Ctx) error {
	campaignID := c.Params("campaignId")
	emailID := c.Params("emailId")
	ip := c.IP()
	userAgent := c.Get("User-Agent")

	log.Printf("[Tracking] Open request: campaign=%s, email=%s, IP=%s", campaignID, emailID, ip)

	// Get tracking service
	ts := getTrackingService()

	// Analyze the request (geolocation + anti-bot)
	trackingInfo := ts.Analyze(ip, userAgent)

	// Log bot detection
	if trackingInfo.IsBot {
		log.Printf("[Tracking] BOT detected: %s (%s) from %s, %s - Score: %d",
			trackingInfo.BotName, trackingInfo.BotType,
			trackingInfo.Country, trackingInfo.City,
			trackingInfo.BotScore)
	} else if trackingInfo.IsSuspicious {
		log.Printf("[Tracking] SUSPICIOUS request from %s, %s - Score: %d",
			trackingInfo.Country, trackingInfo.City,
			trackingInfo.BotScore)
	} else {
		log.Printf("[Tracking] Real open from %s, %s, %s - Browser: %s %s, Device: %s",
			trackingInfo.Country, trackingInfo.Region, trackingInfo.City,
			trackingInfo.Browser, trackingInfo.BrowserVersion,
			trackingInfo.DeviceType)
	}

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

	// Record open event with full tracking info (for analytics) - batched for performance
	if s.batchUpdater != nil {
		s.batchUpdater.AddEvent(tracking.TrackingEvent{
			CampaignID:     campaignID,
			EmailID:        emailID,
			EventType:      "open",
			IPAddress:      ip,
			UserAgent:      userAgent,
			Country:        trackingInfo.Country,
			CountryCode:    trackingInfo.CountryCode,
			Region:         trackingInfo.Region,
			City:           trackingInfo.City,
			Lat:            trackingInfo.Lat,
			Lon:            trackingInfo.Lon,
			Timezone:       trackingInfo.Timezone,
			ISP:            trackingInfo.ISP,
			Browser:        trackingInfo.Browser,
			BrowserVersion: trackingInfo.BrowserVersion,
			OS:             trackingInfo.OS,
			OSVersion:      trackingInfo.OSVersion,
			Device:         trackingInfo.Device,
			DeviceType:     trackingInfo.DeviceType,
			EmailClient:    trackingInfo.EmailClient,
			IsBot:          trackingInfo.IsBot,
			IsSuspicious:   trackingInfo.IsSuspicious,
			BotType:        trackingInfo.BotType,
			BotName:        trackingInfo.BotName,
			BotScore:       trackingInfo.BotScore,
		})
	} else {
		go s.recordEventWithTracking(campaignID, emailID, "open", "", ip, userAgent, trackingInfo)
	}

	// Only count if first open and record exists
	// We count ALL opens (even from bots) - the dashboard can filter later
	if exists && !alreadyOpened {
		log.Printf("[Tracking] Recording first open for campaign=%s, email=%s (bot=%v)", campaignID, emailID, trackingInfo.IsBot)

		// Use batch updater for better performance (updates every 2 seconds)
		if s.batchUpdater != nil {
			s.batchUpdater.AddOpen(campaignID, emailID)
		} else {
			// Fallback to synchronous update
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
		}

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
	ip := c.IP()
	userAgent := c.Get("User-Agent")

	log.Printf("[Tracking] Click request: campaign=%s, email=%s, url=%s, IP=%s", campaignID, emailID, targetURL, ip)

	if targetURL == "" {
		return c.Status(400).SendString("Missing URL")
	}

	// Decode URL
	decodedURL, err := url.QueryUnescape(targetURL)
	if err != nil {
		decodedURL = targetURL
	}

	// Get tracking service
	ts := getTrackingService()

	// Analyze the request (geolocation + anti-bot)
	trackingInfo := ts.Analyze(ip, userAgent)

	// Log click with location info
	if trackingInfo.IsBot {
		log.Printf("[Tracking] BOT click: %s (%s) from %s, %s",
			trackingInfo.BotName, trackingInfo.BotType,
			trackingInfo.Country, trackingInfo.City)
	} else {
		log.Printf("[Tracking] Real click from %s, %s, %s - Browser: %s, Device: %s",
			trackingInfo.Country, trackingInfo.Region, trackingInfo.City,
			trackingInfo.Browser, trackingInfo.DeviceType)
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

	// Record click event (for analytics - all clicks) - batched for performance
	if s.batchUpdater != nil {
		s.batchUpdater.AddEvent(tracking.TrackingEvent{
			CampaignID:     campaignID,
			EmailID:        emailID,
			EventType:      "click",
			LinkURL:        decodedURL,
			IPAddress:      ip,
			UserAgent:      userAgent,
			Country:        trackingInfo.Country,
			CountryCode:    trackingInfo.CountryCode,
			Region:         trackingInfo.Region,
			City:           trackingInfo.City,
			Lat:            trackingInfo.Lat,
			Lon:            trackingInfo.Lon,
			Timezone:       trackingInfo.Timezone,
			ISP:            trackingInfo.ISP,
			Browser:        trackingInfo.Browser,
			BrowserVersion: trackingInfo.BrowserVersion,
			OS:             trackingInfo.OS,
			OSVersion:      trackingInfo.OSVersion,
			Device:         trackingInfo.Device,
			DeviceType:     trackingInfo.DeviceType,
			EmailClient:    trackingInfo.EmailClient,
			IsBot:          trackingInfo.IsBot,
			IsSuspicious:   trackingInfo.IsSuspicious,
			BotType:        trackingInfo.BotType,
			BotName:        trackingInfo.BotName,
			BotScore:       trackingInfo.BotScore,
		})
	} else {
		go s.recordEventWithTracking(campaignID, emailID, "click", decodedURL, ip, userAgent, trackingInfo)
	}

	// Only count if first click and record exists
	if exists && !alreadyClicked {
		log.Printf("[Tracking] Recording first click for campaign=%s, email=%s", campaignID, emailID)

		// Use batch updater for better performance (updates every 2 seconds)
		if s.batchUpdater != nil {
			s.batchUpdater.AddClick(campaignID, emailID)
		} else {
			// Fallback to synchronous update
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
		}

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
	ip := c.IP()
	userAgent := c.Get("User-Agent")

	// Get tracking service for location info
	ts := getTrackingService()
	trackingInfo := ts.Analyze(ip, userAgent)

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

	// Record event with tracking info
	go s.recordEventWithTracking(campaignID, emailID, "unsubscribe", "", ip, userAgent, trackingInfo)

	// Update campaign unsubscribe count
	go s.db.Exec(`
		UPDATE campaigns SET unsubscribe_count = unsubscribe_count + 1 WHERE id = $1
	`, campaignID)

	log.Printf("[Tracking] Unsubscribe from %s, %s - Email: %s", trackingInfo.Country, trackingInfo.City, email)

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

// recordEventWithTracking saves a tracking event with full geolocation and device info
func (s *Server) recordEventWithTracking(campaignID, emailID, eventType, linkURL, ip, userAgent string, info *tracking.TrackingInfo) {
	// Use extended insert with all tracking fields
	_, err := s.db.Exec(`
		INSERT INTO tracking_events (
			id, campaign_id, email_id, event_type, link_url,
			ip_address, user_agent,
			country, country_code, region, city, lat, lon, timezone, isp,
			browser, browser_version, os, os_version, device, device_type, email_client,
			is_bot, is_suspicious, bot_type, bot_name, bot_score
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27)
	`,
		uuid.New().String(), campaignID, emailID, eventType, linkURL,
		ip, userAgent,
		info.Country, info.CountryCode, info.Region, info.City, info.Lat, info.Lon, info.Timezone, info.ISP,
		info.Browser, info.BrowserVersion, info.OS, info.OSVersion, info.Device, info.DeviceType, info.EmailClient,
		info.IsBot, info.IsSuspicious, info.BotType, info.BotName, info.BotScore,
	)

	if err != nil {
		// Fallback to simple insert if new columns don't exist yet
		log.Printf("[Tracking] Extended insert failed, using fallback: %v", err)
		s.recordEvent(campaignID, emailID, eventType, linkURL, ip, userAgent)
	}
}

// recordEvent saves a tracking event (fallback for old schema)
func (s *Server) recordEvent(campaignID, emailID, eventType, linkURL, ip, userAgent string) {
	s.db.Exec(`
		INSERT INTO tracking_events (id, campaign_id, email_id, event_type, link_url, ip_address, user_agent)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, uuid.New().String(), campaignID, emailID, eventType, linkURL, ip, userAgent)
}

// getTrackingStats returns tracking statistics with location breakdown
func (s *Server) getTrackingStats(c *fiber.Ctx) error {
	campaignID := c.Params("id")

	// Get stats by country
	countryStats, err := s.getCountryStats(campaignID)
	if err != nil {
		log.Printf("[Tracking] Error getting country stats: %v", err)
	}

	// Get stats by device type
	deviceStats, err := s.getDeviceStats(campaignID)
	if err != nil {
		log.Printf("[Tracking] Error getting device stats: %v", err)
	}

	// Get stats by browser
	browserStats, err := s.getBrowserStats(campaignID)
	if err != nil {
		log.Printf("[Tracking] Error getting browser stats: %v", err)
	}

	// Get bot vs human stats
	botStats, err := s.getBotStats(campaignID)
	if err != nil {
		log.Printf("[Tracking] Error getting bot stats: %v", err)
	}

	return c.JSON(fiber.Map{
		"countries": countryStats,
		"devices":   deviceStats,
		"browsers":  browserStats,
		"bot_stats": botStats,
	})
}

// getCountryStats returns open/click counts by country
func (s *Server) getCountryStats(campaignID string) ([]map[string]interface{}, error) {
	rows, err := s.db.Query(`
		SELECT
			COALESCE(country, 'Unknown') as country,
			COALESCE(country_code, '') as country_code,
			COUNT(*) as total,
			COUNT(CASE WHEN event_type = 'open' THEN 1 END) as opens,
			COUNT(CASE WHEN event_type = 'click' THEN 1 END) as clicks
		FROM tracking_events
		WHERE campaign_id = $1 AND (is_bot = false OR is_bot IS NULL)
		GROUP BY country, country_code
		ORDER BY total DESC
		LIMIT 20
	`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []map[string]interface{}
	for rows.Next() {
		var country, countryCode string
		var total, opens, clicks int
		if err := rows.Scan(&country, &countryCode, &total, &opens, &clicks); err != nil {
			continue
		}
		stats = append(stats, map[string]interface{}{
			"country":      country,
			"country_code": countryCode,
			"total":        total,
			"opens":        opens,
			"clicks":       clicks,
		})
	}
	return stats, nil
}

// getDeviceStats returns open/click counts by device type
func (s *Server) getDeviceStats(campaignID string) ([]map[string]interface{}, error) {
	rows, err := s.db.Query(`
		SELECT
			COALESCE(device_type, 'unknown') as device_type,
			COUNT(*) as total,
			COUNT(CASE WHEN event_type = 'open' THEN 1 END) as opens,
			COUNT(CASE WHEN event_type = 'click' THEN 1 END) as clicks
		FROM tracking_events
		WHERE campaign_id = $1 AND (is_bot = false OR is_bot IS NULL)
		GROUP BY device_type
		ORDER BY total DESC
	`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []map[string]interface{}
	for rows.Next() {
		var deviceType string
		var total, opens, clicks int
		if err := rows.Scan(&deviceType, &total, &opens, &clicks); err != nil {
			continue
		}
		stats = append(stats, map[string]interface{}{
			"device_type": deviceType,
			"total":       total,
			"opens":       opens,
			"clicks":      clicks,
		})
	}
	return stats, nil
}

// getBrowserStats returns open/click counts by browser
func (s *Server) getBrowserStats(campaignID string) ([]map[string]interface{}, error) {
	rows, err := s.db.Query(`
		SELECT
			COALESCE(browser, 'Unknown') as browser,
			COUNT(*) as total,
			COUNT(CASE WHEN event_type = 'open' THEN 1 END) as opens,
			COUNT(CASE WHEN event_type = 'click' THEN 1 END) as clicks
		FROM tracking_events
		WHERE campaign_id = $1 AND (is_bot = false OR is_bot IS NULL)
		GROUP BY browser
		ORDER BY total DESC
		LIMIT 10
	`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []map[string]interface{}
	for rows.Next() {
		var browser string
		var total, opens, clicks int
		if err := rows.Scan(&browser, &total, &opens, &clicks); err != nil {
			continue
		}
		stats = append(stats, map[string]interface{}{
			"browser": browser,
			"total":   total,
			"opens":   opens,
			"clicks":  clicks,
		})
	}
	return stats, nil
}

// getBotStats returns bot vs human statistics
func (s *Server) getBotStats(campaignID string) (map[string]interface{}, error) {
	var humanOpens, humanClicks, botOpens, botClicks, suspiciousOpens, suspiciousClicks int

	err := s.db.QueryRow(`
		SELECT
			COUNT(CASE WHEN event_type = 'open' AND (is_bot = false OR is_bot IS NULL) AND (is_suspicious = false OR is_suspicious IS NULL) THEN 1 END) as human_opens,
			COUNT(CASE WHEN event_type = 'click' AND (is_bot = false OR is_bot IS NULL) AND (is_suspicious = false OR is_suspicious IS NULL) THEN 1 END) as human_clicks,
			COUNT(CASE WHEN event_type = 'open' AND is_bot = true THEN 1 END) as bot_opens,
			COUNT(CASE WHEN event_type = 'click' AND is_bot = true THEN 1 END) as bot_clicks,
			COUNT(CASE WHEN event_type = 'open' AND is_suspicious = true AND (is_bot = false OR is_bot IS NULL) THEN 1 END) as suspicious_opens,
			COUNT(CASE WHEN event_type = 'click' AND is_suspicious = true AND (is_bot = false OR is_bot IS NULL) THEN 1 END) as suspicious_clicks
		FROM tracking_events
		WHERE campaign_id = $1
	`, campaignID).Scan(&humanOpens, &humanClicks, &botOpens, &botClicks, &suspiciousOpens, &suspiciousClicks)

	if err != nil {
		// If new columns don't exist, return empty stats
		if err == sql.ErrNoRows {
			return map[string]interface{}{
				"human_opens":       0,
				"human_clicks":      0,
				"bot_opens":         0,
				"bot_clicks":        0,
				"suspicious_opens":  0,
				"suspicious_clicks": 0,
			}, nil
		}
		return nil, err
	}

	return map[string]interface{}{
		"human_opens":       humanOpens,
		"human_clicks":      humanClicks,
		"bot_opens":         botOpens,
		"bot_clicks":        botClicks,
		"suspicious_opens":  suspiciousOpens,
		"suspicious_clicks": suspiciousClicks,
	}, nil
}

// getTrackingCityStats returns tracking statistics by city for a specific country
func (s *Server) getTrackingCityStats(c *fiber.Ctx) error {
	campaignID := c.Params("id")
	countryCode := c.Params("countryCode")

	stats, err := s.getCityStats(campaignID, countryCode)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"cities": stats,
	})
}

// getCityStats returns open/click counts by city for a specific country
func (s *Server) getCityStats(campaignID, countryCode string) ([]map[string]interface{}, error) {
	rows, err := s.db.Query(`
		SELECT
			COALESCE(city, 'Unknown') as city,
			COALESCE(region, '') as region,
			COUNT(*) as total,
			COUNT(CASE WHEN event_type = 'open' THEN 1 END) as opens,
			COUNT(CASE WHEN event_type = 'click' THEN 1 END) as clicks
		FROM tracking_events
		WHERE campaign_id = $1
			AND country_code = $2
			AND (is_bot = false OR is_bot IS NULL)
		GROUP BY city, region
		ORDER BY total DESC
		LIMIT 20
	`, campaignID, countryCode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []map[string]interface{}
	for rows.Next() {
		var city, region string
		var total, opens, clicks int
		if err := rows.Scan(&city, &region, &total, &opens, &clicks); err != nil {
			continue
		}
		stats = append(stats, map[string]interface{}{
			"city":   city,
			"region": region,
			"total":  total,
			"opens":  opens,
			"clicks": clicks,
		})
	}
	return stats, nil
}
