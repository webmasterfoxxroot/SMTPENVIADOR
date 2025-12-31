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

	// Get today's stats from Redis
	redisStats, _ := s.queue.GetStats()
	stats["today_sent"] = redisStats["sent"]
	stats["today_failed"] = redisStats["failed"]
	stats["today_opened"] = redisStats["opened"]
	stats["today_clicked"] = redisStats["clicked"]

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

	// Get hourly stats for chart
	rows, _ := s.db.Query(`
		SELECT hour, sent_count, failed_count, open_count, click_count
		FROM stats WHERE date = CURRENT_DATE ORDER BY hour
	`)
	defer rows.Close()

	var hourlyStats []map[string]interface{}
	for rows.Next() {
		var hour, sent, failed, opened, clicked int
		rows.Scan(&hour, &sent, &failed, &opened, &clicked)
		hourlyStats = append(hourlyStats, map[string]interface{}{
			"hour":    hour,
			"sent":    sent,
			"failed":  failed,
			"opened":  opened,
			"clicked": clicked,
		})
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
