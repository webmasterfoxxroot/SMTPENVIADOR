package api

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
)

// getStats returns dashboard statistics
func (s *Server) getStats(c *fiber.Ctx) error {
	stats := make(map[string]interface{})
	userID := getUserID(c)

	// Get period filter (today, week, month)
	period := c.Query("period", "today")

	// Get queue length
	queueLen, _ := s.queue.GetQueueLength()
	stats["queue_size"] = queueLen

	// Define date filter based on period
	var dateFilter string
	switch period {
	case "week":
		dateFilter = "ce.created_at >= CURRENT_DATE - INTERVAL '7 days'"
	case "month":
		dateFilter = "ce.created_at >= CURRENT_DATE - INTERVAL '30 days'"
	default: // today
		dateFilter = "DATE(ce.created_at) = CURRENT_DATE"
	}

	// Get today's stats from database (more reliable than Redis)
	// Sent/Failed today - based on when email was processed, filtered by user's campaigns
	var todaySent, todayFailed int
	s.db.QueryRow(`
		SELECT
			COUNT(CASE WHEN ce.status = 'sent' THEN 1 END),
			COUNT(CASE WHEN ce.status = 'failed' THEN 1 END)
		FROM campaign_emails ce
		JOIN campaigns c ON ce.campaign_id = c.id
		WHERE c.user_id = $1 AND `+dateFilter+`
	`, userID).Scan(&todaySent, &todayFailed)

	// Opened - based on when email was opened
	var todayOpened int
	openedDateFilter := strings.Replace(dateFilter, "ce.created_at", "ce.opened_at", 1)
	s.db.QueryRow(`
		SELECT COUNT(*) FROM campaign_emails ce
		JOIN campaigns c ON ce.campaign_id = c.id
		WHERE c.user_id = $1 AND `+openedDateFilter+`
	`, userID).Scan(&todayOpened)

	// Clicked - based on when link was clicked
	var todayClicked int
	clickedDateFilter := strings.Replace(dateFilter, "ce.created_at", "ce.clicked_at", 1)
	s.db.QueryRow(`
		SELECT COUNT(*) FROM campaign_emails ce
		JOIN campaigns c ON ce.campaign_id = c.id
		WHERE c.user_id = $1 AND `+clickedDateFilter+`
	`, userID).Scan(&todayClicked)

	stats["today_sent"] = todaySent
	stats["today_failed"] = todayFailed
	stats["today_opened"] = todayOpened
	stats["today_clicked"] = todayClicked

	// Get engine stats
	engineStats := s.engine.GetStats()
	stats["sending_rate"] = engineStats["sending_rate"]
	stats["active_workers"] = engineStats["active_workers"]

	// Get active SMTPs for this user (not global engine stats)
	var activeSMTPs int
	s.db.QueryRow(`SELECT COUNT(*) FROM smtp_servers WHERE active = true AND status = 'online' AND user_id = $1`, userID).Scan(&activeSMTPs)
	stats["active_smtps"] = activeSMTPs

	// Get active campaigns count
	var activeCampaigns int
	s.db.QueryRow(`SELECT COUNT(*) FROM campaigns WHERE status = 'running' AND user_id = $1`, userID).Scan(&activeCampaigns)
	stats["active_campaigns"] = activeCampaigns

	// Get total counts (filtered by user)
	var totalSMTPs, totalLists int
	var totalEmails int64
	s.db.QueryRow(`SELECT COUNT(*) FROM smtp_servers WHERE active = true AND user_id = $1`, userID).Scan(&totalSMTPs)
	s.db.QueryRow(`SELECT COUNT(*) FROM email_lists WHERE user_id = $1`, userID).Scan(&totalLists)

	// Get total emails by summing from email_lists table (most accurate)
	// This matches what's shown on the Lists page
	s.db.QueryRow(`SELECT COALESCE(SUM(total_emails), 0) FROM email_lists WHERE user_id = $1`, userID).Scan(&totalEmails)

	// If PostgreSQL sum is 0, try ClickHouse as fallback
	if totalEmails == 0 && s.ch != nil {
		ctx := context.Background()
		// Sum counts from each list in ClickHouse (user's lists only)
		rows, err := s.db.Query(`SELECT id FROM email_lists WHERE user_id = $1`, userID)
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
		// Hourly stats for today (filtered by user's campaigns)
		hourlyRows, _ := s.db.Query(`
			SELECT
				EXTRACT(HOUR FROM ce.sent_at)::int as hour,
				COUNT(*) as sent,
				0 as failed
			FROM campaign_emails ce
			JOIN campaigns c ON ce.campaign_id = c.id
			WHERE c.user_id = $1 AND DATE(ce.sent_at) = CURRENT_DATE AND ce.status = 'sent'
			GROUP BY EXTRACT(HOUR FROM ce.sent_at)
			UNION ALL
			SELECT
				EXTRACT(HOUR FROM ce.created_at)::int as hour,
				0 as sent,
				COUNT(*) as failed
			FROM campaign_emails ce
			JOIN campaigns c ON ce.campaign_id = c.id
			WHERE c.user_id = $1 AND DATE(ce.created_at) = CURRENT_DATE AND ce.status = 'failed'
			GROUP BY EXTRACT(HOUR FROM ce.created_at)
			ORDER BY hour
		`, userID)

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
			SELECT EXTRACT(HOUR FROM ce.opened_at)::int as hour, COUNT(*) as opened
			FROM campaign_emails ce
			JOIN campaigns c ON ce.campaign_id = c.id
			WHERE c.user_id = $1 AND DATE(ce.opened_at) = CURRENT_DATE AND ce.opened_at IS NOT NULL
			GROUP BY EXTRACT(HOUR FROM ce.opened_at)
		`, userID)
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
			SELECT EXTRACT(HOUR FROM ce.clicked_at)::int as hour, COUNT(*) as clicked
			FROM campaign_emails ce
			JOIN campaigns c ON ce.campaign_id = c.id
			WHERE c.user_id = $1 AND DATE(ce.clicked_at) = CURRENT_DATE AND ce.clicked_at IS NOT NULL
			GROUP BY EXTRACT(HOUR FROM ce.clicked_at)
		`, userID)
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
				DATE(ce.sent_at) as day,
				COUNT(CASE WHEN ce.status = 'sent' THEN 1 END) as sent,
				COUNT(CASE WHEN ce.status = 'failed' THEN 1 END) as failed,
				COUNT(CASE WHEN ce.opened_at IS NOT NULL THEN 1 END) as opened,
				COUNT(CASE WHEN ce.clicked_at IS NOT NULL THEN 1 END) as clicked
			FROM campaign_emails ce
			JOIN campaigns c ON ce.campaign_id = c.id
			WHERE c.user_id = $1 AND ce.sent_at >= CURRENT_DATE - INTERVAL '1 day' * $2
			GROUP BY DATE(ce.sent_at)
			ORDER BY day
		`, userID, days)

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

	// Get stats by SMTP provider (today, filtered by user's campaigns)
	smtpRows, _ := s.db.Query(`
		SELECT
			COALESCE(ss.host, 'Desconhecido') as provider,
			COUNT(CASE WHEN ce.status = 'sent' THEN 1 END) as sent,
			COUNT(CASE WHEN ce.status = 'failed' THEN 1 END) as failed,
			COUNT(CASE WHEN ce.opened_at IS NOT NULL THEN 1 END) as opened,
			COUNT(CASE WHEN ce.clicked_at IS NOT NULL THEN 1 END) as clicked
		FROM campaign_emails ce
		JOIN campaigns c ON ce.campaign_id = c.id
		LEFT JOIN smtp_servers ss ON ce.smtp_id = ss.id
		WHERE c.user_id = $1 AND DATE(ce.created_at) = CURRENT_DATE
		GROUP BY ss.host
		ORDER BY sent DESC
		LIMIT 10
	`, userID)
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
	// Use stored email directly from campaign_emails (filtered by user's campaigns)
	domainRows, _ := s.db.Query(`
		SELECT
			LOWER(SPLIT_PART(ce.email, '@', 2)) as domain,
			COUNT(CASE WHEN ce.status = 'sent' THEN 1 END) as sent,
			COUNT(CASE WHEN ce.status = 'failed' THEN 1 END) as failed,
			COUNT(CASE WHEN ce.opened_at IS NOT NULL THEN 1 END) as opened,
			COUNT(CASE WHEN ce.clicked_at IS NOT NULL THEN 1 END) as clicked
		FROM campaign_emails ce
		JOIN campaigns c ON ce.campaign_id = c.id
		WHERE c.user_id = $1 AND DATE(ce.created_at) = CURRENT_DATE AND ce.email IS NOT NULL
		GROUP BY LOWER(SPLIT_PART(ce.email, '@', 2))
		ORDER BY sent DESC
		LIMIT 15
	`, userID)
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
	userID := getUserID(c)

	// Join with campaign_emails to get stored email instead of emails table (filtered by user)
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
		WHERE c.user_id = $1 AND te.event_type IN ('open', 'click') AND ce.email IS NOT NULL
		ORDER BY te.created_at DESC
		LIMIT $2
	`, userID, limit)

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

// getDashboardTrackingStats returns global tracking statistics for dashboard
func (s *Server) getDashboardTrackingStats(c *fiber.Ctx) error {
	userID := getUserID(c)
	period := c.Query("period", "today")

	// Define date filter based on period
	var dateFilter string
	switch period {
	case "week":
		dateFilter = "te.created_at >= CURRENT_DATE - INTERVAL '7 days'"
	case "month":
		dateFilter = "te.created_at >= CURRENT_DATE - INTERVAL '30 days'"
	default: // today
		dateFilter = "DATE(te.created_at) = CURRENT_DATE"
	}

	result := make(map[string]interface{})

	// Check if extended columns exist (migration 007)
	var hasExtendedColumns bool
	s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name = 'tracking_events' AND column_name = 'browser')`).Scan(&hasExtendedColumns)

	// Get stats by country (top 10) - country column always exists
	countryRows, err := s.db.Query(`
		SELECT
			COALESCE(te.country, 'Unknown') as country,
			'' as country_code,
			COUNT(*) as total,
			COUNT(CASE WHEN te.event_type = 'open' THEN 1 END) as opens,
			COUNT(CASE WHEN te.event_type = 'click' THEN 1 END) as clicks
		FROM tracking_events te
		JOIN campaigns c ON te.campaign_id = c.id
		WHERE c.user_id = $1 AND `+dateFilter+`
		GROUP BY te.country
		ORDER BY total DESC
		LIMIT 10
	`, userID)
	if err == nil {
		defer countryRows.Close()
		var countries []map[string]interface{}
		for countryRows.Next() {
			var country, countryCode string
			var total, opens, clicks int
			if countryRows.Scan(&country, &countryCode, &total, &opens, &clicks) == nil {
				countries = append(countries, map[string]interface{}{
					"country":      country,
					"country_code": countryCode,
					"total":        total,
					"opens":        opens,
					"clicks":       clicks,
				})
			}
		}
		result["countries"] = countries
	}

	// Get stats by city (top 10) - city column always exists
	cityRows, err := s.db.Query(`
		SELECT
			COALESCE(te.city, 'Unknown') as city,
			COALESCE(te.country, '') as country,
			COUNT(*) as total,
			COUNT(CASE WHEN te.event_type = 'open' THEN 1 END) as opens,
			COUNT(CASE WHEN te.event_type = 'click' THEN 1 END) as clicks
		FROM tracking_events te
		JOIN campaigns c ON te.campaign_id = c.id
		WHERE c.user_id = $1 AND `+dateFilter+`
		GROUP BY te.city, te.country
		ORDER BY total DESC
		LIMIT 10
	`, userID)
	if err == nil {
		defer cityRows.Close()
		var cities []map[string]interface{}
		for cityRows.Next() {
			var city, country string
			var total, opens, clicks int
			if cityRows.Scan(&city, &country, &total, &opens, &clicks) == nil {
				cities = append(cities, map[string]interface{}{
					"city":    city,
					"country": country,
					"total":   total,
					"opens":   opens,
					"clicks":  clicks,
				})
			}
		}
		result["cities"] = cities
	}

	if hasExtendedColumns {
		// Get stats by region/state (top 10)
		regionRows, err := s.db.Query(`
			SELECT
				COALESCE(te.country, 'Unknown') as country,
				COALESCE(te.region, 'Unknown') as region,
				COUNT(*) as total,
				COUNT(CASE WHEN te.event_type = 'open' THEN 1 END) as opens,
				COUNT(CASE WHEN te.event_type = 'click' THEN 1 END) as clicks
			FROM tracking_events te
			JOIN campaigns c ON te.campaign_id = c.id
			WHERE c.user_id = $1 AND `+dateFilter+` AND (te.is_bot = false OR te.is_bot IS NULL)
			GROUP BY te.country, te.region
			ORDER BY total DESC
			LIMIT 10
		`, userID)
		if err == nil {
			defer regionRows.Close()
			var regions []map[string]interface{}
			for regionRows.Next() {
				var country, region string
				var total, opens, clicks int
				if regionRows.Scan(&country, &region, &total, &opens, &clicks) == nil {
					regions = append(regions, map[string]interface{}{
						"country": country,
						"region":  region,
						"total":   total,
						"opens":   opens,
						"clicks":  clicks,
					})
				}
			}
			result["regions"] = regions
		}

		// Get stats by browser (top 10)
		browserRows, err := s.db.Query(`
			SELECT
				COALESCE(te.browser, 'Unknown') as browser,
				COUNT(*) as total,
				COUNT(CASE WHEN te.event_type = 'open' THEN 1 END) as opens,
				COUNT(CASE WHEN te.event_type = 'click' THEN 1 END) as clicks
			FROM tracking_events te
			JOIN campaigns c ON te.campaign_id = c.id
			WHERE c.user_id = $1 AND `+dateFilter+` AND (te.is_bot = false OR te.is_bot IS NULL)
			GROUP BY te.browser
			ORDER BY total DESC
			LIMIT 10
		`, userID)
		if err == nil {
			defer browserRows.Close()
			var browsers []map[string]interface{}
			for browserRows.Next() {
				var browser string
				var total, opens, clicks int
				if browserRows.Scan(&browser, &total, &opens, &clicks) == nil {
					browsers = append(browsers, map[string]interface{}{
						"browser": browser,
						"total":   total,
						"opens":   opens,
						"clicks":  clicks,
					})
				}
			}
			result["browsers"] = browsers
		}

		// Get stats by device type
		deviceRows, err := s.db.Query(`
			SELECT
				COALESCE(te.device_type, 'unknown') as device_type,
				COUNT(*) as total,
				COUNT(CASE WHEN te.event_type = 'open' THEN 1 END) as opens,
				COUNT(CASE WHEN te.event_type = 'click' THEN 1 END) as clicks
			FROM tracking_events te
			JOIN campaigns c ON te.campaign_id = c.id
			WHERE c.user_id = $1 AND `+dateFilter+` AND (te.is_bot = false OR te.is_bot IS NULL)
			GROUP BY te.device_type
			ORDER BY total DESC
		`, userID)
		if err == nil {
			defer deviceRows.Close()
			var devices []map[string]interface{}
			for deviceRows.Next() {
				var deviceType string
				var total, opens, clicks int
				if deviceRows.Scan(&deviceType, &total, &opens, &clicks) == nil {
					devices = append(devices, map[string]interface{}{
						"device_type": deviceType,
						"total":       total,
						"opens":       opens,
						"clicks":      clicks,
					})
				}
			}
			result["devices"] = devices
		}

		// Get stats by OS
		osRows, err := s.db.Query(`
			SELECT
				COALESCE(te.os, 'Unknown') as os,
				COUNT(*) as total,
				COUNT(CASE WHEN te.event_type = 'open' THEN 1 END) as opens,
				COUNT(CASE WHEN te.event_type = 'click' THEN 1 END) as clicks
			FROM tracking_events te
			JOIN campaigns c ON te.campaign_id = c.id
			WHERE c.user_id = $1 AND `+dateFilter+` AND (te.is_bot = false OR te.is_bot IS NULL)
			GROUP BY te.os
			ORDER BY total DESC
			LIMIT 10
		`, userID)
		if err == nil {
			defer osRows.Close()
			var osList []map[string]interface{}
			for osRows.Next() {
				var osName string
				var total, opens, clicks int
				if osRows.Scan(&osName, &total, &opens, &clicks) == nil {
					osList = append(osList, map[string]interface{}{
						"os":     osName,
						"total":  total,
						"opens":  opens,
						"clicks": clicks,
					})
				}
			}
			result["os"] = osList
		}

		// Get stats by email client
		emailClientRows, err := s.db.Query(`
			SELECT
				COALESCE(te.email_client, 'Unknown') as email_client,
				COUNT(*) as total,
				COUNT(CASE WHEN te.event_type = 'open' THEN 1 END) as opens,
				COUNT(CASE WHEN te.event_type = 'click' THEN 1 END) as clicks
			FROM tracking_events te
			JOIN campaigns c ON te.campaign_id = c.id
			WHERE c.user_id = $1 AND `+dateFilter+` AND (te.is_bot = false OR te.is_bot IS NULL) AND te.email_client IS NOT NULL AND te.email_client != ''
			GROUP BY te.email_client
			ORDER BY total DESC
			LIMIT 10
		`, userID)
		if err == nil {
			defer emailClientRows.Close()
			var emailClients []map[string]interface{}
			for emailClientRows.Next() {
				var emailClient string
				var total, opens, clicks int
				if emailClientRows.Scan(&emailClient, &total, &opens, &clicks) == nil {
					emailClients = append(emailClients, map[string]interface{}{
						"email_client": emailClient,
						"total":        total,
						"opens":        opens,
						"clicks":       clicks,
					})
				}
			}
			result["email_clients"] = emailClients
		}

		// Get bot vs human stats
		var humanOpens, humanClicks, botOpens, botClicks, suspiciousOpens, suspiciousClicks int
		s.db.QueryRow(`
			SELECT
				COUNT(CASE WHEN te.event_type = 'open' AND (te.is_bot = false OR te.is_bot IS NULL) AND (te.is_suspicious = false OR te.is_suspicious IS NULL) THEN 1 END) as human_opens,
				COUNT(CASE WHEN te.event_type = 'click' AND (te.is_bot = false OR te.is_bot IS NULL) AND (te.is_suspicious = false OR te.is_suspicious IS NULL) THEN 1 END) as human_clicks,
				COUNT(CASE WHEN te.event_type = 'open' AND te.is_bot = true THEN 1 END) as bot_opens,
				COUNT(CASE WHEN te.event_type = 'click' AND te.is_bot = true THEN 1 END) as bot_clicks,
				COUNT(CASE WHEN te.event_type = 'open' AND te.is_suspicious = true AND (te.is_bot = false OR te.is_bot IS NULL) THEN 1 END) as suspicious_opens,
				COUNT(CASE WHEN te.event_type = 'click' AND te.is_suspicious = true AND (te.is_bot = false OR te.is_bot IS NULL) THEN 1 END) as suspicious_clicks
			FROM tracking_events te
			JOIN campaigns c ON te.campaign_id = c.id
			WHERE c.user_id = $1 AND `+dateFilter+`
		`, userID).Scan(&humanOpens, &humanClicks, &botOpens, &botClicks, &suspiciousOpens, &suspiciousClicks)

		result["bot_stats"] = map[string]interface{}{
			"human_opens":       humanOpens,
			"human_clicks":      humanClicks,
			"bot_opens":         botOpens,
			"bot_clicks":        botClicks,
			"suspicious_opens":  suspiciousOpens,
			"suspicious_clicks": suspiciousClicks,
			"total_opens":       humanOpens + botOpens + suspiciousOpens,
			"total_clicks":      humanClicks + botClicks + suspiciousClicks,
		}
	} else {
		// Fallback: Parse user_agent to extract browser info
		browserRows, err := s.db.Query(`
			SELECT
				CASE
					WHEN te.user_agent ILIKE '%Edg/%' OR te.user_agent ILIKE '%Edge/%' THEN 'Microsoft Edge'
					WHEN te.user_agent ILIKE '%OPR/%' OR te.user_agent ILIKE '%Opera%' THEN 'Opera'
					WHEN te.user_agent ILIKE '%Vivaldi%' THEN 'Vivaldi'
					WHEN te.user_agent ILIKE '%Brave%' THEN 'Brave'
					WHEN te.user_agent ILIKE '%SamsungBrowser%' THEN 'Samsung Browser'
					WHEN te.user_agent ILIKE '%UCBrowser%' THEN 'UC Browser'
					WHEN te.user_agent ILIKE '%Firefox%' THEN 'Firefox'
					WHEN te.user_agent ILIKE '%Safari%' AND te.user_agent NOT ILIKE '%Chrome%' AND te.user_agent NOT ILIKE '%Chromium%' THEN 'Safari'
					WHEN te.user_agent ILIKE '%Chrome%' OR te.user_agent ILIKE '%Chromium%' THEN 'Chrome'
					WHEN te.user_agent ILIKE '%MSIE%' OR te.user_agent ILIKE '%Trident%' THEN 'Internet Explorer'
					WHEN te.user_agent IS NULL OR te.user_agent = '' THEN 'Unknown'
					ELSE 'Other'
				END as browser,
				COUNT(*) as total,
				COUNT(CASE WHEN te.event_type = 'open' THEN 1 END) as opens,
				COUNT(CASE WHEN te.event_type = 'click' THEN 1 END) as clicks
			FROM tracking_events te
			JOIN campaigns c ON te.campaign_id = c.id
			WHERE c.user_id = $1 AND `+dateFilter+`
			GROUP BY browser
			ORDER BY total DESC
			LIMIT 10
		`, userID)
		if err == nil {
			defer browserRows.Close()
			var browsers []map[string]interface{}
			for browserRows.Next() {
				var browser string
				var total, opens, clicks int
				if browserRows.Scan(&browser, &total, &opens, &clicks) == nil {
					browsers = append(browsers, map[string]interface{}{
						"browser": browser,
						"total":   total,
						"opens":   opens,
						"clicks":  clicks,
					})
				}
			}
			result["browsers"] = browsers
		}

		// Fallback: Parse user_agent to extract device type
		deviceRows, err := s.db.Query(`
			SELECT
				CASE
					WHEN te.user_agent ILIKE '%iPad%' OR te.user_agent ILIKE '%Tablet%' OR te.user_agent ILIKE '%Kindle%' THEN 'tablet'
					WHEN te.user_agent ILIKE '%iPhone%' OR te.user_agent ILIKE '%iPod%' THEN 'mobile'
					WHEN te.user_agent ILIKE '%Mobile%' THEN 'mobile'
					WHEN te.user_agent ILIKE '%Android%' AND te.user_agent NOT ILIKE '%Mobile%' THEN 'tablet'
					WHEN te.user_agent ILIKE '%Android%' THEN 'mobile'
					ELSE 'desktop'
				END as device_type,
				COUNT(*) as total,
				COUNT(CASE WHEN te.event_type = 'open' THEN 1 END) as opens,
				COUNT(CASE WHEN te.event_type = 'click' THEN 1 END) as clicks
			FROM tracking_events te
			JOIN campaigns c ON te.campaign_id = c.id
			WHERE c.user_id = $1 AND `+dateFilter+`
			GROUP BY device_type
			ORDER BY total DESC
		`, userID)
		if err == nil {
			defer deviceRows.Close()
			var devices []map[string]interface{}
			for deviceRows.Next() {
				var deviceType string
				var total, opens, clicks int
				if deviceRows.Scan(&deviceType, &total, &opens, &clicks) == nil {
					devices = append(devices, map[string]interface{}{
						"device_type": deviceType,
						"total":       total,
						"opens":       opens,
						"clicks":      clicks,
					})
				}
			}
			result["devices"] = devices
		}

		// Fallback: Parse user_agent to extract OS
		osRows, err := s.db.Query(`
			SELECT
				CASE
					WHEN te.user_agent ILIKE '%iPhone%' OR te.user_agent ILIKE '%iPad%' OR te.user_agent ILIKE '%iPod%' THEN 'iOS'
					WHEN te.user_agent ILIKE '%Android%' THEN 'Android'
					WHEN te.user_agent ILIKE '%Windows NT 10%' THEN 'Windows 10/11'
					WHEN te.user_agent ILIKE '%Windows NT 6.3%' THEN 'Windows 8.1'
					WHEN te.user_agent ILIKE '%Windows NT 6.2%' THEN 'Windows 8'
					WHEN te.user_agent ILIKE '%Windows NT 6.1%' THEN 'Windows 7'
					WHEN te.user_agent ILIKE '%Windows%' THEN 'Windows'
					WHEN te.user_agent ILIKE '%Mac OS X%' OR te.user_agent ILIKE '%Macintosh%' THEN 'macOS'
					WHEN te.user_agent ILIKE '%Ubuntu%' THEN 'Ubuntu'
					WHEN te.user_agent ILIKE '%Fedora%' THEN 'Fedora'
					WHEN te.user_agent ILIKE '%Linux%' AND te.user_agent NOT ILIKE '%Android%' THEN 'Linux'
					WHEN te.user_agent ILIKE '%CrOS%' THEN 'Chrome OS'
					WHEN te.user_agent IS NULL OR te.user_agent = '' THEN 'Unknown'
					ELSE 'Other'
				END as os,
				COUNT(*) as total,
				COUNT(CASE WHEN te.event_type = 'open' THEN 1 END) as opens,
				COUNT(CASE WHEN te.event_type = 'click' THEN 1 END) as clicks
			FROM tracking_events te
			JOIN campaigns c ON te.campaign_id = c.id
			WHERE c.user_id = $1 AND `+dateFilter+`
			GROUP BY os
			ORDER BY total DESC
			LIMIT 10
		`, userID)
		if err == nil {
			defer osRows.Close()
			var osList []map[string]interface{}
			for osRows.Next() {
				var osName string
				var total, opens, clicks int
				if osRows.Scan(&osName, &total, &opens, &clicks) == nil {
					osList = append(osList, map[string]interface{}{
						"os":     osName,
						"total":  total,
						"opens":  opens,
						"clicks": clicks,
					})
				}
			}
			result["os"] = osList
		}

		// Fallback: Add empty regions if not available
		result["regions"] = []map[string]interface{}{}

		// Basic bot stats without extended columns
		var totalOpens, totalClicks int
		s.db.QueryRow(`
			SELECT
				COUNT(CASE WHEN te.event_type = 'open' THEN 1 END),
				COUNT(CASE WHEN te.event_type = 'click' THEN 1 END)
			FROM tracking_events te
			JOIN campaigns c ON te.campaign_id = c.id
			WHERE c.user_id = $1 AND `+dateFilter+`
		`, userID).Scan(&totalOpens, &totalClicks)

		result["bot_stats"] = map[string]interface{}{
			"human_opens":       totalOpens,
			"human_clicks":      totalClicks,
			"bot_opens":         0,
			"bot_clicks":        0,
			"suspicious_opens":  0,
			"suspicious_clicks": 0,
			"total_opens":       totalOpens,
			"total_clicks":      totalClicks,
		}
	}

	return c.JSON(result)
}

// getCombinedStats returns all dashboard stats in a single request for better performance
func (s *Server) getCombinedStats(c *fiber.Ctx) error {
	userID := getUserID(c)
	period := c.Query("period", "today")

	result := make(map[string]interface{})

	// Get basic stats (same as getStats)
	stats := make(map[string]interface{})

	// Get queue length
	queueLen, _ := s.queue.GetQueueLength()
	stats["queue_size"] = queueLen

	// Define date filter based on period
	var dateFilter string
	switch period {
	case "week":
		dateFilter = "ce.created_at >= CURRENT_DATE - INTERVAL '7 days'"
	case "month":
		dateFilter = "ce.created_at >= CURRENT_DATE - INTERVAL '30 days'"
	default:
		dateFilter = "DATE(ce.created_at) = CURRENT_DATE"
	}

	// Get today's stats
	var todaySent, todayFailed int
	s.db.QueryRow(`
		SELECT
			COUNT(CASE WHEN ce.status = 'sent' THEN 1 END),
			COUNT(CASE WHEN ce.status = 'failed' THEN 1 END)
		FROM campaign_emails ce
		JOIN campaigns c ON ce.campaign_id = c.id
		WHERE c.user_id = $1 AND `+dateFilter+`
	`, userID).Scan(&todaySent, &todayFailed)

	// Opened
	var todayOpened int
	openedDateFilter := strings.Replace(dateFilter, "ce.created_at", "ce.opened_at", 1)
	s.db.QueryRow(`
		SELECT COUNT(*) FROM campaign_emails ce
		JOIN campaigns c ON ce.campaign_id = c.id
		WHERE c.user_id = $1 AND `+openedDateFilter+`
	`, userID).Scan(&todayOpened)

	// Clicked
	var todayClicked int
	clickedDateFilter := strings.Replace(dateFilter, "ce.created_at", "ce.clicked_at", 1)
	s.db.QueryRow(`
		SELECT COUNT(*) FROM campaign_emails ce
		JOIN campaigns c ON ce.campaign_id = c.id
		WHERE c.user_id = $1 AND `+clickedDateFilter+`
	`, userID).Scan(&todayClicked)

	stats["today_sent"] = todaySent
	stats["today_failed"] = todayFailed
	stats["today_opened"] = todayOpened
	stats["today_clicked"] = todayClicked

	// Get engine stats
	engineStats := s.engine.GetStats()
	stats["sending_rate"] = engineStats["sending_rate"]
	stats["active_workers"] = engineStats["active_workers"]

	// Get active SMTPs
	var activeSMTPs int
	s.db.QueryRow(`SELECT COUNT(*) FROM smtp_servers WHERE active = true AND status = 'online' AND user_id = $1`, userID).Scan(&activeSMTPs)
	stats["active_smtps"] = activeSMTPs

	// Get active campaigns
	var activeCampaigns int
	s.db.QueryRow(`SELECT COUNT(*) FROM campaigns WHERE status = 'running' AND user_id = $1`, userID).Scan(&activeCampaigns)
	stats["active_campaigns"] = activeCampaigns

	// Get totals
	var totalSMTPs, totalLists int
	var totalEmails int64
	s.db.QueryRow(`SELECT COUNT(*) FROM smtp_servers WHERE active = true AND user_id = $1`, userID).Scan(&totalSMTPs)
	s.db.QueryRow(`SELECT COUNT(*) FROM email_lists WHERE user_id = $1`, userID).Scan(&totalLists)
	s.db.QueryRow(`SELECT COALESCE(SUM(total_emails), 0) FROM email_lists WHERE user_id = $1`, userID).Scan(&totalEmails)

	stats["total_smtps"] = totalSMTPs
	stats["total_lists"] = totalLists
	stats["total_emails"] = totalEmails

	result["stats"] = stats

	// Get chart stats (hourly/daily)
	var chartStats []map[string]interface{}
	if period == "today" {
		hourlyRows, _ := s.db.Query(`
			SELECT
				EXTRACT(HOUR FROM ce.sent_at)::int as hour,
				COUNT(CASE WHEN ce.status = 'sent' THEN 1 END) as sent,
				COUNT(CASE WHEN ce.status = 'failed' THEN 1 END) as failed,
				COUNT(CASE WHEN ce.opened_at IS NOT NULL THEN 1 END) as opened,
				COUNT(CASE WHEN ce.clicked_at IS NOT NULL THEN 1 END) as clicked
			FROM campaign_emails ce
			JOIN campaigns c ON ce.campaign_id = c.id
			WHERE c.user_id = $1 AND DATE(ce.sent_at) = CURRENT_DATE
			GROUP BY EXTRACT(HOUR FROM ce.sent_at)
			ORDER BY hour
		`, userID)
		if hourlyRows != nil {
			defer hourlyRows.Close()
			for hourlyRows.Next() {
				var hour, sent, failed, opened, clicked int
				hourlyRows.Scan(&hour, &sent, &failed, &opened, &clicked)
				chartStats = append(chartStats, map[string]interface{}{
					"hour":    hour,
					"label":   hour,
					"sent":    sent,
					"failed":  failed,
					"opened":  opened,
					"clicked": clicked,
				})
			}
		}
	} else {
		days := 7
		if period == "month" {
			days = 30
		}
		dailyRows, _ := s.db.Query(`
			SELECT
				DATE(ce.sent_at) as day,
				COUNT(CASE WHEN ce.status = 'sent' THEN 1 END) as sent,
				COUNT(CASE WHEN ce.status = 'failed' THEN 1 END) as failed,
				COUNT(CASE WHEN ce.opened_at IS NOT NULL THEN 1 END) as opened,
				COUNT(CASE WHEN ce.clicked_at IS NOT NULL THEN 1 END) as clicked
			FROM campaign_emails ce
			JOIN campaigns c ON ce.campaign_id = c.id
			WHERE c.user_id = $1 AND ce.sent_at >= CURRENT_DATE - INTERVAL '1 day' * $2
			GROUP BY DATE(ce.sent_at)
			ORDER BY day
		`, userID, days)
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
	result["hourly"] = chartStats

	// Get stats by domain
	domainRows, _ := s.db.Query(`
		SELECT
			LOWER(SPLIT_PART(ce.email, '@', 2)) as domain,
			COUNT(CASE WHEN ce.status = 'sent' THEN 1 END) as sent,
			COUNT(CASE WHEN ce.status = 'failed' THEN 1 END) as failed,
			COUNT(CASE WHEN ce.opened_at IS NOT NULL THEN 1 END) as opened,
			COUNT(CASE WHEN ce.clicked_at IS NOT NULL THEN 1 END) as clicked
		FROM campaign_emails ce
		JOIN campaigns c ON ce.campaign_id = c.id
		WHERE c.user_id = $1 AND DATE(ce.created_at) = CURRENT_DATE AND ce.email IS NOT NULL
		GROUP BY LOWER(SPLIT_PART(ce.email, '@', 2))
		ORDER BY sent DESC
		LIMIT 15
	`, userID)
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
		result["by_domain"] = domainStats
	}

	// Get recent activity
	activityRows, _ := s.db.Query(`
		SELECT
			te.event_type,
			ce.email,
			c.name as campaign_name,
			te.link_url,
			te.created_at
		FROM tracking_events te
		JOIN campaign_emails ce ON te.email_id = ce.email_id AND te.campaign_id = ce.campaign_id
		JOIN campaigns c ON te.campaign_id = c.id
		WHERE c.user_id = $1 AND te.event_type IN ('open', 'click') AND ce.email IS NOT NULL
		ORDER BY te.created_at DESC
		LIMIT 10
	`, userID)
	if activityRows != nil {
		defer activityRows.Close()
		var activities []map[string]interface{}
		for activityRows.Next() {
			var eventType, campaignName string
			var email *string
			var linkURL *string
			var createdAt time.Time
			activityRows.Scan(&eventType, &email, &campaignName, &linkURL, &createdAt)
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
		result["activities"] = activities
	}

	// Include tracking stats inline (for geolocation, browsers, devices)
	trackingDateFilter := strings.Replace(dateFilter, "ce.created_at", "te.created_at", 1)

	// Get stats by country (top 5)
	countryRows, _ := s.db.Query(`
		SELECT
			COALESCE(te.country, 'Unknown') as country,
			COALESCE(te.country_code, '') as country_code,
			COUNT(*) as total,
			COUNT(CASE WHEN te.event_type = 'open' THEN 1 END) as opens,
			COUNT(CASE WHEN te.event_type = 'click' THEN 1 END) as clicks
		FROM tracking_events te
		JOIN campaigns c ON te.campaign_id = c.id
		WHERE c.user_id = $1 AND `+trackingDateFilter+` AND (te.is_bot = false OR te.is_bot IS NULL)
		GROUP BY te.country, te.country_code
		ORDER BY total DESC
		LIMIT 5
	`, userID)
	if countryRows != nil {
		defer countryRows.Close()
		var countries []map[string]interface{}
		for countryRows.Next() {
			var country, countryCode string
			var total, opens, clicks int
			countryRows.Scan(&country, &countryCode, &total, &opens, &clicks)
			countries = append(countries, map[string]interface{}{
				"country":      country,
				"country_code": countryCode,
				"total":        total,
				"opens":        opens,
				"clicks":       clicks,
			})
		}
		result["countries"] = countries
	}

	// Get stats by browser (top 5)
	browserRows, _ := s.db.Query(`
		SELECT
			COALESCE(te.browser, 'Unknown') as browser,
			COUNT(*) as total,
			COUNT(CASE WHEN te.event_type = 'open' THEN 1 END) as opens,
			COUNT(CASE WHEN te.event_type = 'click' THEN 1 END) as clicks
		FROM tracking_events te
		JOIN campaigns c ON te.campaign_id = c.id
		WHERE c.user_id = $1 AND `+trackingDateFilter+` AND (te.is_bot = false OR te.is_bot IS NULL)
		GROUP BY te.browser
		ORDER BY total DESC
		LIMIT 5
	`, userID)
	if browserRows != nil {
		defer browserRows.Close()
		var browsers []map[string]interface{}
		for browserRows.Next() {
			var browser string
			var total, opens, clicks int
			browserRows.Scan(&browser, &total, &opens, &clicks)
			browsers = append(browsers, map[string]interface{}{
				"browser": browser,
				"total":   total,
				"opens":   opens,
				"clicks":  clicks,
			})
		}
		result["browsers"] = browsers
	}

	// Get stats by device type
	deviceRows, _ := s.db.Query(`
		SELECT
			COALESCE(te.device_type, 'unknown') as device_type,
			COUNT(*) as total,
			COUNT(CASE WHEN te.event_type = 'open' THEN 1 END) as opens,
			COUNT(CASE WHEN te.event_type = 'click' THEN 1 END) as clicks
		FROM tracking_events te
		JOIN campaigns c ON te.campaign_id = c.id
		WHERE c.user_id = $1 AND `+trackingDateFilter+` AND (te.is_bot = false OR te.is_bot IS NULL)
		GROUP BY te.device_type
		ORDER BY total DESC
	`, userID)
	if deviceRows != nil {
		defer deviceRows.Close()
		var devices []map[string]interface{}
		for deviceRows.Next() {
			var deviceType string
			var total, opens, clicks int
			deviceRows.Scan(&deviceType, &total, &opens, &clicks)
			devices = append(devices, map[string]interface{}{
				"device_type": deviceType,
				"total":       total,
				"opens":       opens,
				"clicks":      clicks,
			})
		}
		result["devices"] = devices
	}

	// Get stats by OS (top 5)
	osRows, _ := s.db.Query(`
		SELECT
			COALESCE(te.os, 'Unknown') as os,
			COUNT(*) as total,
			COUNT(CASE WHEN te.event_type = 'open' THEN 1 END) as opens,
			COUNT(CASE WHEN te.event_type = 'click' THEN 1 END) as clicks
		FROM tracking_events te
		JOIN campaigns c ON te.campaign_id = c.id
		WHERE c.user_id = $1 AND `+trackingDateFilter+` AND (te.is_bot = false OR te.is_bot IS NULL)
		GROUP BY te.os
		ORDER BY total DESC
		LIMIT 5
	`, userID)
	if osRows != nil {
		defer osRows.Close()
		var osList []map[string]interface{}
		for osRows.Next() {
			var osName string
			var total, opens, clicks int
			osRows.Scan(&osName, &total, &opens, &clicks)
			osList = append(osList, map[string]interface{}{
				"os":     osName,
				"total":  total,
				"opens":  opens,
				"clicks": clicks,
			})
		}
		result["os"] = osList
	}

	// Get stats by region/state (top 5)
	regionRows, _ := s.db.Query(`
		SELECT
			COALESCE(te.country, 'Unknown') as country,
			COALESCE(te.region, 'Unknown') as region,
			COUNT(*) as total,
			COUNT(CASE WHEN te.event_type = 'open' THEN 1 END) as opens,
			COUNT(CASE WHEN te.event_type = 'click' THEN 1 END) as clicks
		FROM tracking_events te
		JOIN campaigns c ON te.campaign_id = c.id
		WHERE c.user_id = $1 AND `+trackingDateFilter+` AND (te.is_bot = false OR te.is_bot IS NULL)
		GROUP BY te.country, te.region
		ORDER BY total DESC
		LIMIT 5
	`, userID)
	if regionRows != nil {
		defer regionRows.Close()
		var regions []map[string]interface{}
		for regionRows.Next() {
			var country, region string
			var total, opens, clicks int
			regionRows.Scan(&country, &region, &total, &opens, &clicks)
			regions = append(regions, map[string]interface{}{
				"country": country,
				"region":  region,
				"total":   total,
				"opens":   opens,
				"clicks":  clicks,
			})
		}
		result["regions"] = regions
	}

	// Get stats by city (top 5)
	cityRows, _ := s.db.Query(`
		SELECT
			COALESCE(te.city, 'Unknown') as city,
			COALESCE(te.country, '') as country,
			COUNT(*) as total,
			COUNT(CASE WHEN te.event_type = 'open' THEN 1 END) as opens,
			COUNT(CASE WHEN te.event_type = 'click' THEN 1 END) as clicks
		FROM tracking_events te
		JOIN campaigns c ON te.campaign_id = c.id
		WHERE c.user_id = $1 AND `+trackingDateFilter+` AND (te.is_bot = false OR te.is_bot IS NULL)
		GROUP BY te.city, te.country
		ORDER BY total DESC
		LIMIT 5
	`, userID)
	if cityRows != nil {
		defer cityRows.Close()
		var cities []map[string]interface{}
		for cityRows.Next() {
			var city, country string
			var total, opens, clicks int
			cityRows.Scan(&city, &country, &total, &opens, &clicks)
			cities = append(cities, map[string]interface{}{
				"city":    city,
				"country": country,
				"total":   total,
				"opens":   opens,
				"clicks":  clicks,
			})
		}
		result["cities"] = cities
	}

	// Get bot stats summary
	var humanOpens, humanClicks, botOpens, botClicks int
	s.db.QueryRow(`
		SELECT
			COUNT(CASE WHEN te.event_type = 'open' AND (te.is_bot = false OR te.is_bot IS NULL) THEN 1 END) as human_opens,
			COUNT(CASE WHEN te.event_type = 'click' AND (te.is_bot = false OR te.is_bot IS NULL) THEN 1 END) as human_clicks,
			COUNT(CASE WHEN te.event_type = 'open' AND te.is_bot = true THEN 1 END) as bot_opens,
			COUNT(CASE WHEN te.event_type = 'click' AND te.is_bot = true THEN 1 END) as bot_clicks
		FROM tracking_events te
		JOIN campaigns c ON te.campaign_id = c.id
		WHERE c.user_id = $1 AND `+trackingDateFilter+`
	`, userID).Scan(&humanOpens, &humanClicks, &botOpens, &botClicks)

	result["bot_stats"] = map[string]interface{}{
		"human_opens":  humanOpens,
		"human_clicks": humanClicks,
		"bot_opens":    botOpens,
		"bot_clicks":   botClicks,
	}

	return c.JSON(result)
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
