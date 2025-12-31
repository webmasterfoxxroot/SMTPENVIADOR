package api

import (
	"encoding/json"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
)

// getStats returns dashboard statistics
func (s *Server) getStats(c *fiber.Ctx) error {
	stats := make(map[string]interface{})

	// Get queue length
	queueLen, _ := s.queue.GetQueueLength()
	stats["queue_size"] = queueLen

	// Get today's stats from database (more reliable than Redis)
	// Sent/Failed today - based on when email was processed
	var todaySent, todayFailed int
	s.db.QueryRow(`
		SELECT
			COUNT(CASE WHEN status = 'sent' THEN 1 END),
			COUNT(CASE WHEN status = 'failed' THEN 1 END)
		FROM campaign_emails
		WHERE DATE(created_at) = CURRENT_DATE
	`).Scan(&todaySent, &todayFailed)

	// Opened today - based on when email was opened (can be from any campaign)
	var todayOpened int
	s.db.QueryRow(`
		SELECT COUNT(*) FROM campaign_emails
		WHERE DATE(opened_at) = CURRENT_DATE
	`).Scan(&todayOpened)

	// Clicked today - based on when link was clicked (can be from any campaign)
	var todayClicked int
	s.db.QueryRow(`
		SELECT COUNT(*) FROM campaign_emails
		WHERE DATE(clicked_at) = CURRENT_DATE
	`).Scan(&todayClicked)

	stats["today_sent"] = todaySent
	stats["today_failed"] = todayFailed
	stats["today_opened"] = todayOpened
	stats["today_clicked"] = todayClicked

	// Get engine stats
	engineStats := s.engine.GetStats()
	stats["sending_rate"] = engineStats["sending_rate"]
	stats["active_workers"] = engineStats["active_workers"]
	stats["active_smtps"] = engineStats["active_smtps"]

	// Get active campaigns count
	var activeCampaigns int
	s.db.QueryRow(`SELECT COUNT(*) FROM campaigns WHERE status = 'running'`).Scan(&activeCampaigns)
	stats["active_campaigns"] = activeCampaigns

	// Get total counts
	var totalSMTPs, totalLists, totalEmails int
	s.db.QueryRow(`SELECT COUNT(*) FROM smtp_servers WHERE active = true`).Scan(&totalSMTPs)
	s.db.QueryRow(`SELECT COUNT(*) FROM email_lists`).Scan(&totalLists)
	s.db.QueryRow(`SELECT COUNT(*) FROM emails WHERE valid = true`).Scan(&totalEmails)

	stats["total_smtps"] = totalSMTPs
	stats["total_lists"] = totalLists
	stats["total_emails"] = totalEmails

	// Get hourly stats for chart - from campaign_emails table
	hourlyRows, _ := s.db.Query(`
		SELECT
			EXTRACT(HOUR FROM sent_at)::int as hour,
			COUNT(*) as sent,
			0 as failed
		FROM campaign_emails
		WHERE DATE(sent_at) = CURRENT_DATE AND status = 'sent'
		GROUP BY EXTRACT(HOUR FROM sent_at)
		UNION ALL
		SELECT
			EXTRACT(HOUR FROM created_at)::int as hour,
			0 as sent,
			COUNT(*) as failed
		FROM campaign_emails
		WHERE DATE(created_at) = CURRENT_DATE AND status = 'failed'
		GROUP BY EXTRACT(HOUR FROM created_at)
		ORDER BY hour
	`)

	// Aggregate hourly data
	hourlyMap := make(map[int]map[string]int)
	if hourlyRows != nil {
		defer hourlyRows.Close()
		for hourlyRows.Next() {
			var hour, sent, failed int
			hourlyRows.Scan(&hour, &sent, &failed)
			if _, exists := hourlyMap[hour]; !exists {
				hourlyMap[hour] = map[string]int{"sent": 0, "failed": 0, "opened": 0, "clicked": 0}
			}
			hourlyMap[hour]["sent"] += sent
			hourlyMap[hour]["failed"] += failed
		}
	}

	// Get opens by hour
	openRows, _ := s.db.Query(`
		SELECT EXTRACT(HOUR FROM opened_at)::int as hour, COUNT(*) as opened
		FROM campaign_emails
		WHERE DATE(opened_at) = CURRENT_DATE AND opened_at IS NOT NULL
		GROUP BY EXTRACT(HOUR FROM opened_at)
	`)
	if openRows != nil {
		defer openRows.Close()
		for openRows.Next() {
			var hour, opened int
			openRows.Scan(&hour, &opened)
			if _, exists := hourlyMap[hour]; !exists {
				hourlyMap[hour] = map[string]int{"sent": 0, "failed": 0, "opened": 0, "clicked": 0}
			}
			hourlyMap[hour]["opened"] = opened
		}
	}

	// Get clicks by hour
	clickRows, _ := s.db.Query(`
		SELECT EXTRACT(HOUR FROM clicked_at)::int as hour, COUNT(*) as clicked
		FROM campaign_emails
		WHERE DATE(clicked_at) = CURRENT_DATE AND clicked_at IS NOT NULL
		GROUP BY EXTRACT(HOUR FROM clicked_at)
	`)
	if clickRows != nil {
		defer clickRows.Close()
		for clickRows.Next() {
			var hour, clicked int
			clickRows.Scan(&hour, &clicked)
			if _, exists := hourlyMap[hour]; !exists {
				hourlyMap[hour] = map[string]int{"sent": 0, "failed": 0, "opened": 0, "clicked": 0}
			}
			hourlyMap[hour]["clicked"] = clicked
		}
	}

	// Convert map to slice
	var hourlyStats []map[string]interface{}
	for hour := 0; hour < 24; hour++ {
		if data, exists := hourlyMap[hour]; exists {
			hourlyStats = append(hourlyStats, map[string]interface{}{
				"hour":    hour,
				"sent":    data["sent"],
				"failed":  data["failed"],
				"opened":  data["opened"],
				"clicked": data["clicked"],
			})
		}
	}
	stats["hourly"] = hourlyStats

	// Get stats by SMTP provider (today)
	smtpRows, _ := s.db.Query(`
		SELECT
			COALESCE(ss.host, 'Desconhecido') as provider,
			COUNT(CASE WHEN ce.status = 'sent' THEN 1 END) as sent,
			COUNT(CASE WHEN ce.status = 'failed' THEN 1 END) as failed,
			COUNT(CASE WHEN ce.opened_at IS NOT NULL THEN 1 END) as opened,
			COUNT(CASE WHEN ce.clicked_at IS NOT NULL THEN 1 END) as clicked
		FROM campaign_emails ce
		LEFT JOIN smtp_servers ss ON ce.smtp_id = ss.id
		WHERE DATE(ce.created_at) = CURRENT_DATE
		GROUP BY ss.host
		ORDER BY sent DESC
		LIMIT 10
	`)
	if smtpRows != nil {
		defer smtpRows.Close()

		var providerStats []map[string]interface{}
		for smtpRows.Next() {
			var provider string
			var sent, failed, opened, clicked int
			smtpRows.Scan(&provider, &sent, &failed, &opened, &clicked)
			providerStats = append(providerStats, map[string]interface{}{
				"provider": provider,
				"sent":     sent,
				"failed":   failed,
				"opened":   opened,
				"clicked":  clicked,
			})
		}
		stats["by_provider"] = providerStats
	}

	// Get stats by email domain (today) - gmail.com, hotmail.com, etc.
	domainRows, _ := s.db.Query(`
		SELECT
			LOWER(SPLIT_PART(e.email, '@', 2)) as domain,
			COUNT(CASE WHEN ce.status = 'sent' THEN 1 END) as sent,
			COUNT(CASE WHEN ce.status = 'failed' THEN 1 END) as failed,
			COUNT(CASE WHEN ce.opened_at IS NOT NULL THEN 1 END) as opened,
			COUNT(CASE WHEN ce.clicked_at IS NOT NULL THEN 1 END) as clicked
		FROM campaign_emails ce
		JOIN emails e ON ce.email_id = e.id
		WHERE DATE(ce.created_at) = CURRENT_DATE
		GROUP BY LOWER(SPLIT_PART(e.email, '@', 2))
		ORDER BY sent DESC
		LIMIT 15
	`)
	if domainRows != nil {
		defer domainRows.Close()

		var domainStats []map[string]interface{}
		for domainRows.Next() {
			var domain string
			var sent, failed, opened, clicked int
			domainRows.Scan(&domain, &sent, &failed, &opened, &clicked)
			domainStats = append(domainStats, map[string]interface{}{
				"domain":  domain,
				"sent":    sent,
				"failed":  failed,
				"opened":  opened,
				"clicked": clicked,
			})
		}
		stats["by_domain"] = domainStats
	}

	return c.JSON(stats)
}

// realtimeStats sends real-time stats via WebSocket
func (s *Server) realtimeStats(c *websocket.Conn) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Get current stats
			engineStats := s.engine.GetStats()
			queueLen, _ := s.queue.GetQueueLength()
			redisStats, _ := s.queue.GetStats()

			stats := map[string]interface{}{
				"sending_rate":   engineStats["sending_rate"],
				"total_sent":     engineStats["total_sent"],
				"total_failed":   engineStats["total_failed"],
				"active_workers": engineStats["active_workers"],
				"queue_size":     queueLen,
				"today_sent":     redisStats["sent"],
				"today_failed":   redisStats["failed"],
				"timestamp":      time.Now().Unix(),
			}

			data, _ := json.Marshal(stats)
			if err := c.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		}
	}
}

// getLogs returns system logs
func (s *Server) getLogs(c *fiber.Ctx) error {
	level := c.Query("level", "")
	limit := c.QueryInt("limit", 100)
	offset := c.QueryInt("offset", 0)

	query := `SELECT id, level, message, details, created_at FROM logs`
	args := []interface{}{}

	if level != "" {
		query += " WHERE level = $1"
		args = append(args, level)
	}

	query += " ORDER BY created_at DESC LIMIT $" + string(rune(len(args)+1+'0')) + " OFFSET $" + string(rune(len(args)+2+'0'))
	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch logs"})
	}
	defer rows.Close()

	var logs []fiber.Map
	for rows.Next() {
		var id, logLevel, message string
		var details *string
		var createdAt time.Time

		rows.Scan(&id, &logLevel, &message, &details, &createdAt)
		logs = append(logs, fiber.Map{
			"id":         id,
			"level":      logLevel,
			"message":    message,
			"details":    details,
			"created_at": createdAt,
		})
	}

	return c.JSON(fiber.Map{
		"data":  logs,
		"total": len(logs),
	})
}
