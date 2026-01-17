package api

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
)

// getStats returns dashboard statistics
func (s *Server) getStats(c *fiber.Ctx) error {
	stats := make(map[string]interface{})

	// Get period filter (today, week, month)
	period := c.Query("period", "today")

	// Get queue length
	queueLen, _ := s.queue.GetQueueLength()
	stats["queue_size"] = queueLen

	// Define date filter based on period
	var dateFilter string
	switch period {
	case "week":
		dateFilter = "created_at >= CURRENT_DATE - INTERVAL '7 days'"
	case "month":
		dateFilter = "created_at >= CURRENT_DATE - INTERVAL '30 days'"
	default: // today
		dateFilter = "DATE(created_at) = CURRENT_DATE"
	}

	// Get today's stats from database (more reliable than Redis)
	// Sent/Failed today - based on when email was processed
	var todaySent, todayFailed int
	s.db.QueryRow(`
		SELECT
			COUNT(CASE WHEN status = 'sent' THEN 1 END),
			COUNT(CASE WHEN status = 'failed' THEN 1 END)
		FROM campaign_emails
		WHERE ` + dateFilter + `
	`).Scan(&todaySent, &todayFailed)

	// Opened - based on when email was opened
	var todayOpened int
	openedDateFilter := strings.Replace(dateFilter, "created_at", "opened_at", 1)
	s.db.QueryRow(`
		SELECT COUNT(*) FROM campaign_emails
		WHERE ` + openedDateFilter + `
	`).Scan(&todayOpened)

	// Clicked - based on when link was clicked
	var todayClicked int
	clickedDateFilter := strings.Replace(dateFilter, "created_at", "clicked_at", 1)
	s.db.QueryRow(`
		SELECT COUNT(*) FROM campaign_emails
		WHERE ` + clickedDateFilter + `
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
	var totalSMTPs, totalLists int
	var totalEmails int64
	s.db.QueryRow(`SELECT COUNT(*) FROM smtp_servers WHERE active = true`).Scan(&totalSMTPs)
	s.db.QueryRow(`SELECT COUNT(*) FROM email_lists`).Scan(&totalLists)

	// Get total emails by summing from email_lists table (most accurate)
	// This matches what's shown on the Lists page
	s.db.QueryRow(`SELECT COALESCE(SUM(total_emails), 0) FROM email_lists`).Scan(&totalEmails)

	// If PostgreSQL sum is 0, try ClickHouse as fallback
	if totalEmails == 0 && s.ch != nil {
		ctx := context.Background()
		// Sum counts from each list in ClickHouse
		rows, err := s.db.Query(`SELECT id FROM email_lists`)
		if err == nil {
			defer rows.Close()
			var sumCount uint64
			for rows.Next() {
				var listID string
				if rows.Scan(&listID) == nil {
					count, err := s.ch.GetEmailCountByList(ctx, listID)
					if err == nil {
						sumCount += count
					}
				}
			}
			if sumCount > 0 {
				totalEmails = int64(sumCount)
			}
		}
	}

	stats["total_smtps"] = totalSMTPs
	stats["total_lists"] = totalLists
	stats["total_emails"] = totalEmails

	// Get chart stats - hourly for today, daily for week/month
	var chartStats []map[string]interface{}

	if period == "today" {
		// Hourly stats for today
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

		for hour := 0; hour < 24; hour++ {
			if data, exists := hourlyMap[hour]; exists {
				chartStats = append(chartStats, map[string]interface{}{
					"hour":    hour,
					"label":   hour,
					"sent":    data["sent"],
					"failed":  data["failed"],
					"opened":  data["opened"],
					"clicked": data["clicked"],
				})
			}
		}
	} else {
		// Daily stats for week/month
		days := 7
		if period == "month" {
			days = 30
		}

		dailyRows, _ := s.db.Query(`
			SELECT
				DATE(sent_at) as day,
				COUNT(CASE WHEN status = 'sent' THEN 1 END) as sent,
				COUNT(CASE WHEN status = 'failed' THEN 1 END) as failed,
				COUNT(CASE WHEN opened_at IS NOT NULL THEN 1 END) as opened,
				COUNT(CASE WHEN clicked_at IS NOT NULL THEN 1 END) as clicked
			FROM campaign_emails
			WHERE sent_at >= CURRENT_DATE - INTERVAL '1 day' * $1
			GROUP BY DATE(sent_at)
			ORDER BY day
		`, days)

		if dailyRows != nil {
			defer dailyRows.Close()
			for dailyRows.Next() {
				var day time.Time
				var sent, failed, opened, clicked int
				dailyRows.Scan(&day, &sent, &failed, &opened, &clicked)
				chartStats = append(chartStats, map[string]interface{}{
					"hour":    day.Day(),
					"label":   day.Format("02/01"),
					"sent":    sent,
					"failed":  failed,
					"opened":  opened,
					"clicked": clicked,
				})
			}
		}
	}
	stats["hourly"] = chartStats

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
	// Use stored email directly from campaign_emails
	domainRows, _ := s.db.Query(`
		SELECT
			LOWER(SPLIT_PART(ce.email, '@', 2)) as domain,
			COUNT(CASE WHEN ce.status = 'sent' THEN 1 END) as sent,
			COUNT(CASE WHEN ce.status = 'failed' THEN 1 END) as failed,
			COUNT(CASE WHEN ce.opened_at IS NOT NULL THEN 1 END) as opened,
			COUNT(CASE WHEN ce.clicked_at IS NOT NULL THEN 1 END) as clicked
		FROM campaign_emails ce
		WHERE DATE(ce.created_at) = CURRENT_DATE AND ce.email IS NOT NULL
		GROUP BY LOWER(SPLIT_PART(ce.email, '@', 2))
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

// getRecentActivity returns recent opens and clicks for live feed
func (s *Server) getRecentActivity(c *fiber.Ctx) error {
	limit := c.QueryInt("limit", 20)

	// Join with campaign_emails to get stored email instead of emails table
	rows, err := s.db.Query(`
		SELECT
			te.event_type,
			ce.email,
			c.name as campaign_name,
			te.link_url,
			te.created_at
		FROM tracking_events te
		JOIN campaign_emails ce ON te.email_id = ce.email_id AND te.campaign_id = ce.campaign_id
		JOIN campaigns c ON te.campaign_id = c.id
		WHERE te.event_type IN ('open', 'click') AND ce.email IS NOT NULL
		ORDER BY te.created_at DESC
		LIMIT $1
	`, limit)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch activity"})
	}
	defer rows.Close()

	var activities []map[string]interface{}
	for rows.Next() {
		var eventType, campaignName string
		var email *string
		var linkURL *string
		var createdAt time.Time

		rows.Scan(&eventType, &email, &campaignName, &linkURL, &createdAt)

		emailStr := ""
		if email != nil {
			emailStr = *email
		}

		activity := map[string]interface{}{
			"type":      eventType,
			"email":     emailStr,
			"campaign":  campaignName,
			"timestamp": createdAt,
		}
		if linkURL != nil {
			activity["link"] = *linkURL
		}
		activities = append(activities, activity)
	}

	return c.JSON(fiber.Map{
		"activities": activities,
	})
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
