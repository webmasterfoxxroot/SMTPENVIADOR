package api

import (
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/smtp"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// ============================================
// DATABASE MIGRATIONS
// ============================================

func (s *Server) initWarmupTables() {
	// Check if migration needed
	var needsMigration bool

	// Check if warmup_smtps has wrong column type (VARCHAR instead of UUID)
	var columnType string
	err := s.db.QueryRow(`
		SELECT data_type FROM information_schema.columns
		WHERE table_name = 'warmup_smtps' AND column_name = 'id'
	`).Scan(&columnType)
	if err == nil && columnType == "character varying" {
		needsMigration = true
	}

	// Check if warmup_seeds has wrong column type
	err = s.db.QueryRow(`
		SELECT data_type FROM information_schema.columns
		WHERE table_name = 'warmup_seeds' AND column_name = 'id'
	`).Scan(&columnType)
	if err == nil && columnType == "character varying" {
		needsMigration = true
	}

	// Check if warmup_emails table exists (might have failed to create)
	var emailsTableExists bool
	s.db.QueryRow(`
		SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = 'warmup_emails')
	`).Scan(&emailsTableExists)

	// Also check if warmup_smtps exists but warmup_emails doesn't
	var smtpsTableExists bool
	s.db.QueryRow(`
		SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = 'warmup_smtps')
	`).Scan(&smtpsTableExists)

	if smtpsTableExists && !emailsTableExists {
		needsMigration = true
	}

	if needsMigration {
		log.Println("[Warmup] Migrating tables to UUID schema...")
		// Drop ALL warmup tables in correct order due to foreign keys
		s.db.Exec(`DROP TABLE IF EXISTS warmup_daily_stats CASCADE`)
		s.db.Exec(`DROP TABLE IF EXISTS warmup_emails CASCADE`)
		s.db.Exec(`DROP TABLE IF EXISTS warmup_templates CASCADE`)
		s.db.Exec(`DROP TABLE IF EXISTS warmup_seeds CASCADE`)
		s.db.Exec(`DROP TABLE IF EXISTS warmup_smtps CASCADE`)
	}

	// Create warmup_smtps table
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS warmup_smtps (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			smtp_id UUID NOT NULL REFERENCES smtp_servers(id) ON DELETE CASCADE,
			status VARCHAR(20) DEFAULT 'active',
			recipe_type VARCHAR(20) DEFAULT 'progressive',
			start_date TIMESTAMP DEFAULT NOW(),
			end_date TIMESTAMP,
			current_day INT DEFAULT 1,
			min_emails_per_day INT DEFAULT 5,
			max_emails_per_day INT DEFAULT 40,
			reply_rate INT DEFAULT 30,
			start_hour INT DEFAULT 8,
			end_hour INT DEFAULT 18,
			total_sent INT DEFAULT 0,
			total_inbox INT DEFAULT 0,
			total_spam INT DEFAULT 0,
			total_replies INT DEFAULT 0,
			custom_schedule TEXT,
			created_at TIMESTAMP DEFAULT NOW(),
			updated_at TIMESTAMP DEFAULT NOW()
		)
	`)
	if err != nil {
		log.Printf("[Warmup] Error creating warmup_smtps table: %v", err)
	}

	// Create warmup_seeds table
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS warmup_seeds (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			email VARCHAR(255) NOT NULL UNIQUE,
			password VARCHAR(255) NOT NULL,
			provider VARCHAR(50) DEFAULT 'other',
			imap_host VARCHAR(255) NOT NULL,
			imap_port INT DEFAULT 993,
			smtp_host VARCHAR(255),
			smtp_port INT DEFAULT 587,
			use_tls BOOLEAN DEFAULT true,
			status VARCHAR(20) DEFAULT 'active',
			last_check TIMESTAMP,
			error_message TEXT,
			created_at TIMESTAMP DEFAULT NOW(),
			updated_at TIMESTAMP DEFAULT NOW()
		)
	`)
	if err != nil {
		log.Printf("[Warmup] Error creating warmup_seeds table: %v", err)
	}

	// Create warmup_emails table
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS warmup_emails (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			warmup_smtp_id UUID NOT NULL REFERENCES warmup_smtps(id) ON DELETE CASCADE,
			seed_id UUID NOT NULL REFERENCES warmup_seeds(id) ON DELETE CASCADE,
			subject VARCHAR(500),
			message_id VARCHAR(255),
			status VARCHAR(20) DEFAULT 'sent',
			landed_in_spam BOOLEAN DEFAULT false,
			moved_to_inbox BOOLEAN DEFAULT false,
			sent_at TIMESTAMP DEFAULT NOW(),
			opened_at TIMESTAMP,
			replied_at TIMESTAMP,
			created_at TIMESTAMP DEFAULT NOW()
		)
	`)
	if err != nil {
		log.Printf("[Warmup] Error creating warmup_emails table: %v", err)
	}

	// Create warmup_templates table
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS warmup_templates (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			subject VARCHAR(500) NOT NULL,
			body TEXT NOT NULL,
			category VARCHAR(50) DEFAULT 'business',
			active BOOLEAN DEFAULT true,
			created_at TIMESTAMP DEFAULT NOW()
		)
	`)
	if err != nil {
		log.Printf("[Warmup] Error creating warmup_templates table: %v", err)
	}

	// Create warmup_daily_stats table
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS warmup_daily_stats (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			warmup_smtp_id UUID NOT NULL REFERENCES warmup_smtps(id) ON DELETE CASCADE,
			date DATE NOT NULL,
			scheduled INT DEFAULT 0,
			sent INT DEFAULT 0,
			inbox INT DEFAULT 0,
			spam INT DEFAULT 0,
			replies INT DEFAULT 0,
			reply_percent FLOAT DEFAULT 0,
			UNIQUE(warmup_smtp_id, date)
		)
	`)
	if err != nil {
		log.Printf("[Warmup] Error creating warmup_daily_stats table: %v", err)
	}

	// Insert default warmup templates
	s.insertDefaultWarmupTemplates()

	log.Println("[Warmup] Tables initialized")
}

func (s *Server) insertDefaultWarmupTemplates() {
	var count int
	s.db.QueryRow(`SELECT COUNT(*) FROM warmup_templates`).Scan(&count)
	if count > 0 {
		return
	}

	templates := []struct {
		subject  string
		body     string
		category string
	}{
		// Business templates
		{"Quick question about your services", "Hi,\n\nI came across your company and wanted to reach out. Do you have a few minutes to discuss potential collaboration?\n\nBest regards", "business"},
		{"Following up on our conversation", "Hello,\n\nI wanted to follow up on our previous discussion. Have you had a chance to review the information I sent?\n\nLooking forward to hearing from you.", "business"},
		{"Partnership opportunity", "Hi there,\n\nI believe there might be a great opportunity for us to work together. Would you be open to a brief call this week?\n\nThanks!", "business"},
		{"Meeting request", "Hello,\n\nI would like to schedule a meeting to discuss some ideas. What does your availability look like next week?\n\nBest", "business"},
		{"Introduction", "Hi,\n\nMy name is {{name}} and I'm reaching out because I think we could benefit from connecting. Let me know if you'd be interested in chatting.\n\nRegards", "business"},

		// Casual templates
		{"Hey, quick question", "Hey!\n\nHope you're doing well. I had a quick question - do you have a moment?\n\nThanks!", "casual"},
		{"Checking in", "Hi there,\n\nJust wanted to check in and see how things are going. Let me know if you need anything!\n\nCheers", "casual"},
		{"Thought of you", "Hey,\n\nI saw something today that made me think of you. Hope everything is going great on your end!\n\nTalk soon", "casual"},
		{"Long time no talk", "Hi!\n\nIt's been a while since we last connected. How have you been? Would love to catch up sometime.\n\nBest", "casual"},
		{"Quick update", "Hey,\n\nJust wanted to give you a quick update on things. Let me know when you have a few minutes to chat.\n\nThanks!", "casual"},

		// Newsletter style
		{"Weekly digest", "Hello,\n\nHere's your weekly roundup of industry news and updates. Check out the highlights below.\n\nStay informed!", "newsletter"},
		{"Don't miss out", "Hi,\n\nWe have some exciting updates to share with you. Take a look when you get a chance!\n\nBest regards", "newsletter"},
		{"Your monthly summary", "Hello,\n\nHere's a summary of what happened this month. Some great progress has been made!\n\nCheers", "newsletter"},
		{"New features available", "Hi there,\n\nWe've just released some new features that you might find interesting. Check them out!\n\nThanks for your continued support", "newsletter"},
		{"Important announcement", "Hello,\n\nWe have an important announcement to share with you. Please take a moment to read through.\n\nThank you!", "newsletter"},
	}

	for _, t := range templates {
		s.db.Exec(`
			INSERT INTO warmup_templates (id, subject, body, category, active)
			VALUES ($1, $2, $3, $4, true)
		`, uuid.New().String(), t.subject, t.body, t.category)
	}

	log.Println("[Warmup] Default templates inserted")
}

// ============================================
// WARMUP SMTP HANDLERS
// ============================================

// listWarmupSMTPs returns all SMTPs enrolled in warmup
func (s *Server) listWarmupSMTPs(c *fiber.Ctx) error {
	rows, err := s.db.Query(`
		SELECT
			w.id, w.smtp_id, s.name as smtp_name, w.status, w.recipe_type,
			w.start_date, w.end_date, w.current_day,
			w.min_emails_per_day, w.max_emails_per_day, w.reply_rate,
			w.start_hour, w.end_hour,
			w.total_sent, w.total_inbox, w.total_spam, w.total_replies,
			w.custom_schedule, w.created_at, w.updated_at
		FROM warmup_smtps w
		JOIN smtp_servers s ON w.smtp_id = s.id
		ORDER BY w.created_at DESC
	`)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	var smtps []fiber.Map
	for rows.Next() {
		var id, smtpID, smtpName, status, recipeType string
		var startDate time.Time
		var endDate sql.NullTime
		var currentDay, minEmails, maxEmails, replyRate, startHour, endHour int
		var totalSent, totalInbox, totalSpam, totalReplies int
		var customSchedule sql.NullString
		var createdAt, updatedAt time.Time

		err := rows.Scan(
			&id, &smtpID, &smtpName, &status, &recipeType,
			&startDate, &endDate, &currentDay,
			&minEmails, &maxEmails, &replyRate,
			&startHour, &endHour,
			&totalSent, &totalInbox, &totalSpam, &totalReplies,
			&customSchedule, &createdAt, &updatedAt,
		)
		if err != nil {
			continue
		}

		smtp := fiber.Map{
			"id":                 id,
			"smtp_id":            smtpID,
			"smtp_name":          smtpName,
			"status":             status,
			"recipe_type":        recipeType,
			"start_date":         startDate,
			"current_day":        currentDay,
			"min_emails_per_day": minEmails,
			"max_emails_per_day": maxEmails,
			"reply_rate":         replyRate,
			"start_hour":         startHour,
			"end_hour":           endHour,
			"total_sent":         totalSent,
			"total_inbox":        totalInbox,
			"total_spam":         totalSpam,
			"total_replies":      totalReplies,
			"created_at":         createdAt,
			"updated_at":         updatedAt,
		}

		if endDate.Valid {
			smtp["end_date"] = endDate.Time
		}
		if customSchedule.Valid {
			smtp["custom_schedule"] = customSchedule.String
		}

		// Calculate inbox rate
		if totalSent > 0 {
			smtp["inbox_rate"] = float64(totalInbox) / float64(totalSent) * 100
			smtp["spam_rate"] = float64(totalSpam) / float64(totalSent) * 100
		} else {
			smtp["inbox_rate"] = 0.0
			smtp["spam_rate"] = 0.0
		}

		smtps = append(smtps, smtp)
	}

	if smtps == nil {
		smtps = []fiber.Map{}
	}

	return c.JSON(smtps)
}

// createWarmupSMTP adds an SMTP to warmup program
func (s *Server) createWarmupSMTP(c *fiber.Ctx) error {
	var req struct {
		SMTPID          string `json:"smtp_id"`
		RecipeType      string `json:"recipe_type"`
		StartDate       string `json:"start_date"`
		EndDate         string `json:"end_date"`
		MinEmailsPerDay int    `json:"min_emails_per_day"`
		MaxEmailsPerDay int    `json:"max_emails_per_day"`
		ReplyRate       int    `json:"reply_rate"`
		StartHour       int    `json:"start_hour"`
		EndHour         int    `json:"end_hour"`
		CustomSchedule  []int  `json:"custom_schedule"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// Validate SMTP exists
	var exists bool
	s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM smtp_servers WHERE id = $1)`, req.SMTPID).Scan(&exists)
	if !exists {
		return c.Status(404).JSON(fiber.Map{"error": "SMTP not found"})
	}

	// Check if already enrolled
	s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM warmup_smtps WHERE smtp_id = $1)`, req.SMTPID).Scan(&exists)
	if exists {
		return c.Status(400).JSON(fiber.Map{"error": "SMTP already enrolled in warmup"})
	}

	// Set defaults
	if req.RecipeType == "" {
		req.RecipeType = "progressive"
	}
	if req.MinEmailsPerDay == 0 {
		req.MinEmailsPerDay = 5
	}
	if req.MaxEmailsPerDay == 0 {
		req.MaxEmailsPerDay = 40
	}
	if req.ReplyRate == 0 {
		req.ReplyRate = 30
	}
	if req.StartHour == 0 {
		req.StartHour = 8
	}
	if req.EndHour == 0 {
		req.EndHour = 18
	}

	id := uuid.New().String()

	// Parse dates
	startDate := time.Now()
	if req.StartDate != "" {
		if parsed, err := time.Parse("2006-01-02", req.StartDate); err == nil {
			startDate = parsed
		}
	}

	var endDate *time.Time
	if req.EndDate != "" {
		if parsed, err := time.Parse("2006-01-02", req.EndDate); err == nil {
			endDate = &parsed
		}
	}

	// Convert custom schedule to JSON
	var customScheduleJSON sql.NullString
	if len(req.CustomSchedule) > 0 {
		if jsonBytes, err := json.Marshal(req.CustomSchedule); err == nil {
			customScheduleJSON = sql.NullString{String: string(jsonBytes), Valid: true}
		}
	}

	_, err := s.db.Exec(`
		INSERT INTO warmup_smtps (
			id, smtp_id, status, recipe_type, start_date, end_date,
			min_emails_per_day, max_emails_per_day, reply_rate,
			start_hour, end_hour, custom_schedule
		) VALUES ($1, $2, 'active', $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, id, req.SMTPID, req.RecipeType, startDate, endDate,
		req.MinEmailsPerDay, req.MaxEmailsPerDay, req.ReplyRate,
		req.StartHour, req.EndHour, customScheduleJSON)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// Generate initial schedule if progressive
	if req.RecipeType == "progressive" {
		s.generateProgressiveSchedule(id, req.MinEmailsPerDay, req.MaxEmailsPerDay, startDate, endDate)
	}

	return c.JSON(fiber.Map{"id": id, "message": "SMTP enrolled in warmup"})
}

// updateWarmupSMTP updates warmup settings for an SMTP
func (s *Server) updateWarmupSMTP(c *fiber.Ctx) error {
	id := c.Params("id")

	var req struct {
		Status          string `json:"status"`
		RecipeType      string `json:"recipe_type"`
		MinEmailsPerDay int    `json:"min_emails_per_day"`
		MaxEmailsPerDay int    `json:"max_emails_per_day"`
		ReplyRate       int    `json:"reply_rate"`
		StartHour       int    `json:"start_hour"`
		EndHour         int    `json:"end_hour"`
		CustomSchedule  []int  `json:"custom_schedule"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// Convert custom schedule to JSON
	var customScheduleJSON sql.NullString
	if len(req.CustomSchedule) > 0 {
		if jsonBytes, err := json.Marshal(req.CustomSchedule); err == nil {
			customScheduleJSON = sql.NullString{String: string(jsonBytes), Valid: true}
		}
	}

	_, err := s.db.Exec(`
		UPDATE warmup_smtps SET
			status = COALESCE(NULLIF($1, ''), status),
			recipe_type = COALESCE(NULLIF($2, ''), recipe_type),
			min_emails_per_day = CASE WHEN $3 > 0 THEN $3 ELSE min_emails_per_day END,
			max_emails_per_day = CASE WHEN $4 > 0 THEN $4 ELSE max_emails_per_day END,
			reply_rate = CASE WHEN $5 > 0 THEN $5 ELSE reply_rate END,
			start_hour = CASE WHEN $6 >= 0 THEN $6 ELSE start_hour END,
			end_hour = CASE WHEN $7 > 0 THEN $7 ELSE end_hour END,
			custom_schedule = COALESCE($8, custom_schedule),
			updated_at = NOW()
		WHERE id = $9
	`, req.Status, req.RecipeType, req.MinEmailsPerDay, req.MaxEmailsPerDay,
		req.ReplyRate, req.StartHour, req.EndHour, customScheduleJSON, id)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Warmup settings updated"})
}

// deleteWarmupSMTP removes an SMTP from warmup program
func (s *Server) deleteWarmupSMTP(c *fiber.Ctx) error {
	id := c.Params("id")

	_, err := s.db.Exec(`DELETE FROM warmup_smtps WHERE id = $1`, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "SMTP removed from warmup"})
}

// toggleWarmupSMTP toggles warmup on/off for an SMTP
func (s *Server) toggleWarmupSMTP(c *fiber.Ctx) error {
	id := c.Params("id")

	var currentStatus string
	err := s.db.QueryRow(`SELECT status FROM warmup_smtps WHERE id = $1`, id).Scan(&currentStatus)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Warmup SMTP not found"})
	}

	newStatus := "active"
	if currentStatus == "active" {
		newStatus = "paused"
	}

	s.db.Exec(`UPDATE warmup_smtps SET status = $1, updated_at = NOW() WHERE id = $2`, newStatus, id)

	return c.JSON(fiber.Map{"status": newStatus})
}

// triggerWarmupSMTP manually sends warmup emails for testing
func (s *Server) triggerWarmupSMTP(c *fiber.Ctx) error {
	id := c.Params("id")

	// Get number of emails to send (default 1)
	count := c.QueryInt("count", 1)
	if count < 1 {
		count = 1
	}
	if count > 10 {
		count = 10 // Max 10 at a time for safety
	}

	// Get SMTP and warmup info
	var smtpID, recipeType string
	var host, username, password, tlsMode string
	var port, replyRate int

	err := s.db.QueryRow(`
		SELECT w.smtp_id, w.recipe_type, w.reply_rate,
			   s.host, s.port, s.username, s.password, s.tls_mode
		FROM warmup_smtps w
		JOIN smtp_servers s ON w.smtp_id = s.id
		WHERE w.id = $1
	`, id).Scan(&smtpID, &recipeType, &replyRate, &host, &port, &username, &password, &tlsMode)

	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Warmup SMTP not found"})
	}

	// Check for active seeds
	var activeSeeds int
	s.db.QueryRow(`SELECT COUNT(*) FROM warmup_seeds WHERE status = 'active'`).Scan(&activeSeeds)
	if activeSeeds == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "No active seed accounts available"})
	}

	// Send emails
	sent := 0
	errors := []string{}
	for i := 0; i < count; i++ {
		err := s.sendWarmupEmailWithResult(id, smtpID, host, port, username, password, tlsMode, replyRate)
		if err != nil {
			errors = append(errors, err.Error())
		} else {
			sent++
		}
	}

	result := fiber.Map{
		"sent":      sent,
		"requested": count,
		"message":   fmt.Sprintf("Sent %d of %d warmup emails", sent, count),
	}
	if len(errors) > 0 {
		result["errors"] = errors
	}

	return c.JSON(result)
}

// sendWarmupEmailWithResult sends a warmup email and returns error if any
func (s *Server) sendWarmupEmailWithResult(warmupID, smtpID, host string, port int, username, password, tlsMode string, replyRate int) error {
	// Get a random active seed
	var seedID, seedEmail string
	err := s.db.QueryRow(`
		SELECT id, email FROM warmup_seeds
		WHERE status = 'active'
		ORDER BY RANDOM()
		LIMIT 1
	`).Scan(&seedID, &seedEmail)

	if err != nil {
		return fmt.Errorf("no active seeds available")
	}

	// Get a random sender from smtp_senders (or fall back to username)
	var senderEmail string
	var senderName sql.NullString
	err = s.db.QueryRow(`
		SELECT email, name FROM smtp_senders
		WHERE smtp_id = $1 AND active = true
		ORDER BY RANDOM()
		LIMIT 1
	`, smtpID).Scan(&senderEmail, &senderName)

	if err != nil || senderEmail == "" {
		// Fall back to SMTP username if no senders configured
		senderEmail = username
	}

	// Format From address with name if available
	fromAddress := senderEmail
	if senderName.Valid && senderName.String != "" {
		fromAddress = fmt.Sprintf("%s <%s>", senderName.String, senderEmail)
	}

	// Get a random template
	var subject, body string
	err = s.db.QueryRow(`
		SELECT subject, body FROM warmup_templates
		WHERE active = true
		ORDER BY RANDOM()
		LIMIT 1
	`).Scan(&subject, &body)

	if err != nil {
		return fmt.Errorf("no templates available")
	}

	// Add some randomization to subject
	subject = subject + " #" + fmt.Sprintf("%d", rand.Intn(9999))

	// Generate message ID
	messageID := fmt.Sprintf("<%s@warmup>", uuid.New().String())

	// Send email via SMTP
	err = s.sendSMTPEmail(host, port, username, password, tlsMode, fromAddress, seedEmail, subject, body, messageID)
	if err != nil {
		return fmt.Errorf("SMTP error: %v", err)
	}

	// Record the email
	emailID := uuid.New().String()
	s.db.Exec(`
		INSERT INTO warmup_emails (id, warmup_smtp_id, seed_id, subject, message_id, status, sent_at)
		VALUES ($1, $2, $3, $4, $5, 'sent', NOW())
	`, emailID, warmupID, seedID, subject, messageID)

	// Update stats
	s.db.Exec(`
		UPDATE warmup_smtps SET total_sent = total_sent + 1, updated_at = NOW()
		WHERE id = $1
	`, warmupID)

	// Update daily stats
	s.updateWarmupDailyStats(warmupID, "sent")

	// Update sender stats
	s.db.Exec(`UPDATE smtp_senders SET total_sent = total_sent + 1 WHERE email = $1 AND smtp_id = $2`, senderEmail, smtpID)

	log.Printf("[Warmup Manual] ✉️ Sent from %s to %s: %s", senderEmail, seedEmail, subject)
	return nil
}

// getWarmupSMTPStats gets detailed stats for a warmup SMTP
func (s *Server) getWarmupSMTPStats(c *fiber.Ctx) error {
	id := c.Params("id")

	// Get daily stats for the last 30 days
	rows, err := s.db.Query(`
		SELECT date, scheduled, sent, inbox, spam, replies, reply_percent
		FROM warmup_daily_stats
		WHERE warmup_smtp_id = $1
		ORDER BY date DESC
		LIMIT 30
	`, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	var dailyStats []fiber.Map
	for rows.Next() {
		var date time.Time
		var scheduled, sent, inbox, spam, replies int
		var replyPercent float64

		rows.Scan(&date, &scheduled, &sent, &inbox, &spam, &replies, &replyPercent)

		dailyStats = append(dailyStats, fiber.Map{
			"date":          date.Format("2006-01-02"),
			"scheduled":     scheduled,
			"sent":          sent,
			"inbox":         inbox,
			"spam":          spam,
			"replies":       replies,
			"reply_percent": replyPercent,
		})
	}

	// Get recent emails
	emailRows, _ := s.db.Query(`
		SELECT e.id, e.subject, e.status, e.landed_in_spam, e.sent_at, s.email as seed_email
		FROM warmup_emails e
		JOIN warmup_seeds s ON e.seed_id = s.id
		WHERE e.warmup_smtp_id = $1
		ORDER BY e.sent_at DESC
		LIMIT 20
	`, id)
	defer emailRows.Close()

	var recentEmails []fiber.Map
	for emailRows.Next() {
		var emailID, subject, status, seedEmail string
		var landedInSpam bool
		var sentAt time.Time

		emailRows.Scan(&emailID, &subject, &status, &landedInSpam, &sentAt, &seedEmail)

		recentEmails = append(recentEmails, fiber.Map{
			"id":             emailID,
			"subject":        subject,
			"status":         status,
			"landed_in_spam": landedInSpam,
			"sent_at":        sentAt,
			"seed_email":     seedEmail,
		})
	}

	return c.JSON(fiber.Map{
		"daily_stats":   dailyStats,
		"recent_emails": recentEmails,
	})
}

// updateWarmupSchedule updates the custom schedule (from draggable chart)
func (s *Server) updateWarmupSchedule(c *fiber.Ctx) error {
	id := c.Params("id")

	var req struct {
		Schedule []int `json:"schedule"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	jsonBytes, _ := json.Marshal(req.Schedule)

	_, err := s.db.Exec(`
		UPDATE warmup_smtps
		SET custom_schedule = $1, recipe_type = 'custom', updated_at = NOW()
		WHERE id = $2
	`, string(jsonBytes), id)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Schedule updated"})
}

// ============================================
// WARMUP SEED HANDLERS
// ============================================

// listWarmupSeeds returns all seed accounts with statistics
func (s *Server) listWarmupSeeds(c *fiber.Ctx) error {
	rows, err := s.db.Query(`
		SELECT
			ws.id, ws.email, ws.provider, ws.imap_host, ws.imap_port, ws.smtp_host, ws.smtp_port,
			ws.use_tls, ws.status, ws.last_check, ws.error_message, ws.created_at,
			COALESCE(stats.total_received, 0) as total_received,
			COALESCE(stats.total_inbox, 0) as total_inbox,
			COALESCE(stats.total_spam, 0) as total_spam,
			COALESCE(stats.total_moved, 0) as total_moved,
			COALESCE(stats.total_replied, 0) as total_replied
		FROM warmup_seeds ws
		LEFT JOIN (
			SELECT
				seed_id,
				COUNT(*) as total_received,
				COUNT(CASE WHEN landed_in_spam = false THEN 1 END) as total_inbox,
				COUNT(CASE WHEN landed_in_spam = true THEN 1 END) as total_spam,
				COUNT(CASE WHEN moved_to_inbox = true THEN 1 END) as total_moved,
				COUNT(CASE WHEN replied_at IS NOT NULL THEN 1 END) as total_replied
			FROM warmup_emails
			GROUP BY seed_id
		) stats ON ws.id = stats.seed_id
		ORDER BY ws.created_at DESC
	`)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	var seeds []fiber.Map
	for rows.Next() {
		var id, email, provider, imapHost, smtpHost, status string
		var imapPort, smtpPort int
		var useTLS bool
		var lastCheck sql.NullTime
		var errorMsg sql.NullString
		var createdAt time.Time
		var totalReceived, totalInbox, totalSpam, totalMoved, totalReplied int

		rows.Scan(&id, &email, &provider, &imapHost, &imapPort, &smtpHost, &smtpPort,
			&useTLS, &status, &lastCheck, &errorMsg, &createdAt,
			&totalReceived, &totalInbox, &totalSpam, &totalMoved, &totalReplied)

		seed := fiber.Map{
			"id":             id,
			"email":          email,
			"provider":       provider,
			"imap_host":      imapHost,
			"imap_port":      imapPort,
			"smtp_host":      smtpHost,
			"smtp_port":      smtpPort,
			"use_tls":        useTLS,
			"status":         status,
			"created_at":     createdAt,
			"total_received": totalReceived,
			"total_inbox":    totalInbox,
			"total_spam":     totalSpam,
			"total_moved":    totalMoved,
			"total_replied":  totalReplied,
		}

		if lastCheck.Valid {
			seed["last_check"] = lastCheck.Time
		}
		if errorMsg.Valid {
			seed["error_message"] = errorMsg.String
		}

		seeds = append(seeds, seed)
	}

	if seeds == nil {
		seeds = []fiber.Map{}
	}

	return c.JSON(seeds)
}

// createWarmupSeed adds a new seed account
func (s *Server) createWarmupSeed(c *fiber.Ctx) error {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Provider string `json:"provider"`
		IMAPHost string `json:"imap_host"`
		IMAPPort int    `json:"imap_port"`
		SMTPHost string `json:"smtp_host"`
		SMTPPort int    `json:"smtp_port"`
		UseTLS   bool   `json:"use_tls"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// Auto-detect provider settings if not provided
	if req.Provider == "" || req.IMAPHost == "" {
		req.Provider, req.IMAPHost, req.IMAPPort, req.SMTPHost, req.SMTPPort = detectProviderSettings(req.Email)
	}

	if req.IMAPPort == 0 {
		req.IMAPPort = 993
	}
	if req.SMTPPort == 0 {
		req.SMTPPort = 587
	}

	id := uuid.New().String()

	_, err := s.db.Exec(`
		INSERT INTO warmup_seeds (id, email, password, provider, imap_host, imap_port, smtp_host, smtp_port, use_tls, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'active')
	`, id, req.Email, req.Password, req.Provider, req.IMAPHost, req.IMAPPort, req.SMTPHost, req.SMTPPort, req.UseTLS)

	if err != nil {
		if strings.Contains(err.Error(), "unique") {
			return c.Status(400).JSON(fiber.Map{"error": "Email already exists"})
		}
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// Test connection
	go s.testSeedConnection(id)

	return c.JSON(fiber.Map{"id": id, "message": "Seed account added"})
}

// deleteSeed removes a seed account
func (s *Server) deleteWarmupSeed(c *fiber.Ctx) error {
	id := c.Params("id")

	_, err := s.db.Exec(`DELETE FROM warmup_seeds WHERE id = $1`, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Seed removed"})
}

// testWarmupSeed tests IMAP connection for a seed
func (s *Server) testWarmupSeed(c *fiber.Ctx) error {
	id := c.Params("id")

	var email, password, imapHost string
	var imapPort int
	var useTLS bool

	err := s.db.QueryRow(`
		SELECT email, password, imap_host, imap_port, use_tls
		FROM warmup_seeds WHERE id = $1
	`, id).Scan(&email, &password, &imapHost, &imapPort, &useTLS)

	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Seed not found"})
	}

	// Test IMAP connection
	err = testIMAPConnection(imapHost, imapPort, email, password, useTLS)
	if err != nil {
		s.db.Exec(`UPDATE warmup_seeds SET status = 'error', error_message = $1, last_check = NOW() WHERE id = $2`,
			err.Error(), id)
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	s.db.Exec(`UPDATE warmup_seeds SET status = 'active', error_message = NULL, last_check = NOW() WHERE id = $1`, id)

	return c.JSON(fiber.Map{"message": "Connection successful"})
}

// ============================================
// WARMUP STATS HANDLERS
// ============================================

// getWarmupStats returns overall warmup statistics
func (s *Server) getWarmupStats(c *fiber.Ctx) error {
	var totalSMTPs, activeSMTPs, totalSeeds, activeSeeds int
	var totalSent, totalInbox, totalSpam, totalReplies int

	s.db.QueryRow(`SELECT COUNT(*) FROM warmup_smtps`).Scan(&totalSMTPs)
	s.db.QueryRow(`SELECT COUNT(*) FROM warmup_smtps WHERE status = 'active'`).Scan(&activeSMTPs)
	s.db.QueryRow(`SELECT COUNT(*) FROM warmup_seeds`).Scan(&totalSeeds)
	s.db.QueryRow(`SELECT COUNT(*) FROM warmup_seeds WHERE status = 'active'`).Scan(&activeSeeds)

	s.db.QueryRow(`
		SELECT COALESCE(SUM(total_sent), 0), COALESCE(SUM(total_inbox), 0),
			   COALESCE(SUM(total_spam), 0), COALESCE(SUM(total_replies), 0)
		FROM warmup_smtps
	`).Scan(&totalSent, &totalInbox, &totalSpam, &totalReplies)

	var spamRate, inboxRate float64
	if totalSent > 0 {
		spamRate = float64(totalSpam) / float64(totalSent) * 100
		inboxRate = float64(totalInbox) / float64(totalSent) * 100
	}

	return c.JSON(fiber.Map{
		"total_smtps":       totalSMTPs,
		"active_smtps":      activeSMTPs,
		"total_seeds":       totalSeeds,
		"active_seeds":      activeSeeds,
		"total_sent":        totalSent,
		"total_interactions": totalInbox + totalReplies,
		"total_replies":     totalReplies,
		"spam_rate":         spamRate,
		"inbox_rate":        inboxRate,
	})
}

// getWarmupActivity returns recent warmup activity
func (s *Server) getWarmupActivity(c *fiber.Ctx) error {
	rows, err := s.db.Query(`
		SELECT e.id, e.subject, e.status, e.sent_at, s.email as seed_email,
			   sm.name as smtp_name
		FROM warmup_emails e
		JOIN warmup_seeds s ON e.seed_id = s.id
		JOIN warmup_smtps w ON e.warmup_smtp_id = w.id
		JOIN smtp_servers sm ON w.smtp_id = sm.id
		ORDER BY e.sent_at DESC
		LIMIT 50
	`)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	var activities []fiber.Map
	for rows.Next() {
		var id, subject, status, seedEmail, smtpName string
		var sentAt time.Time

		rows.Scan(&id, &subject, &status, &sentAt, &seedEmail, &smtpName)

		activities = append(activities, fiber.Map{
			"id":         id,
			"subject":    subject,
			"status":     status,
			"sent_at":    sentAt,
			"seed_email": seedEmail,
			"smtp_name":  smtpName,
		})
	}

	if activities == nil {
		activities = []fiber.Map{}
	}

	return c.JSON(activities)
}

// ============================================
// WARMUP TEMPLATES HANDLERS
// ============================================

// listWarmupTemplates returns all warmup templates
func (s *Server) listWarmupTemplates(c *fiber.Ctx) error {
	rows, err := s.db.Query(`
		SELECT id, subject, body, category, active, created_at
		FROM warmup_templates
		ORDER BY category, created_at
	`)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	var templates []fiber.Map
	for rows.Next() {
		var id, subject, body, category string
		var active bool
		var createdAt time.Time

		rows.Scan(&id, &subject, &body, &category, &active, &createdAt)

		templates = append(templates, fiber.Map{
			"id":         id,
			"subject":    subject,
			"body":       body,
			"category":   category,
			"active":     active,
			"created_at": createdAt,
		})
	}

	if templates == nil {
		templates = []fiber.Map{}
	}

	return c.JSON(templates)
}

// createWarmupTemplate creates a new warmup template
func (s *Server) createWarmupTemplate(c *fiber.Ctx) error {
	var req struct {
		Subject  string `json:"subject"`
		Body     string `json:"body"`
		Category string `json:"category"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.Category == "" {
		req.Category = "business"
	}

	id := uuid.New().String()

	_, err := s.db.Exec(`
		INSERT INTO warmup_templates (id, subject, body, category, active)
		VALUES ($1, $2, $3, $4, true)
	`, id, req.Subject, req.Body, req.Category)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"id": id, "message": "Template created"})
}

// deleteWarmupTemplate deletes a warmup template
func (s *Server) deleteWarmupTemplate(c *fiber.Ctx) error {
	id := c.Params("id")

	_, err := s.db.Exec(`DELETE FROM warmup_templates WHERE id = $1`, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Template deleted"})
}

// ============================================
// HELPER FUNCTIONS
// ============================================

func detectProviderSettings(email string) (provider, imapHost string, imapPort int, smtpHost string, smtpPort int) {
	domain := strings.ToLower(strings.Split(email, "@")[1])

	switch {
	case strings.Contains(domain, "gmail") || strings.Contains(domain, "googlemail"):
		return "gmail", "imap.gmail.com", 993, "smtp.gmail.com", 587
	case strings.Contains(domain, "yahoo"):
		return "yahoo", "imap.mail.yahoo.com", 993, "smtp.mail.yahoo.com", 587
	case strings.Contains(domain, "outlook") || strings.Contains(domain, "hotmail") || strings.Contains(domain, "live"):
		return "outlook", "outlook.office365.com", 993, "smtp.office365.com", 587
	case strings.Contains(domain, "aol"):
		return "aol", "imap.aol.com", 993, "smtp.aol.com", 587
	case strings.Contains(domain, "icloud") || strings.Contains(domain, "me.com") || strings.Contains(domain, "mac.com"):
		return "icloud", "imap.mail.me.com", 993, "smtp.mail.me.com", 587
	case strings.Contains(domain, "gmx"):
		return "gmx", "imap.gmx.com", 993, "mail.gmx.com", 587
	default:
		return "other", "imap." + domain, 993, "smtp." + domain, 587
	}
}

func testIMAPConnection(host string, port int, email, password string, useTLS bool) error {
	addr := fmt.Sprintf("%s:%d", host, port)

	var c *client.Client
	var err error

	if useTLS || port == 993 {
		c, err = client.DialTLS(addr, &tls.Config{ServerName: host})
	} else {
		c, err = client.Dial(addr)
	}

	if err != nil {
		return fmt.Errorf("failed to connect: %v", err)
	}
	defer c.Logout()

	if err := c.Login(email, password); err != nil {
		return fmt.Errorf("login failed: %v", err)
	}

	return nil
}

func (s *Server) testSeedConnection(seedID string) {
	var email, password, imapHost string
	var imapPort int
	var useTLS bool

	err := s.db.QueryRow(`
		SELECT email, password, imap_host, imap_port, use_tls
		FROM warmup_seeds WHERE id = $1
	`, seedID).Scan(&email, &password, &imapHost, &imapPort, &useTLS)

	if err != nil {
		return
	}

	err = testIMAPConnection(imapHost, imapPort, email, password, useTLS)
	if err != nil {
		s.db.Exec(`UPDATE warmup_seeds SET status = 'error', error_message = $1, last_check = NOW() WHERE id = $2`,
			err.Error(), seedID)
	} else {
		s.db.Exec(`UPDATE warmup_seeds SET status = 'active', error_message = NULL, last_check = NOW() WHERE id = $1`, seedID)
	}
}

func (s *Server) generateProgressiveSchedule(warmupID string, minEmails, maxEmails int, startDate time.Time, endDate *time.Time) {
	// Calculate number of days
	days := 45
	if endDate != nil {
		days = int(endDate.Sub(startDate).Hours() / 24)
	}

	// Generate progressive schedule
	schedule := make([]int, days)
	increment := float64(maxEmails-minEmails) / float64(days-1)

	for i := 0; i < days; i++ {
		schedule[i] = minEmails + int(float64(i)*increment)
		if schedule[i] > maxEmails {
			schedule[i] = maxEmails
		}
	}

	jsonBytes, _ := json.Marshal(schedule)
	s.db.Exec(`UPDATE warmup_smtps SET custom_schedule = $1 WHERE id = $2`, string(jsonBytes), warmupID)
}

// ============================================
// WARMUP ENGINE (Background Process)
// ============================================

func (s *Server) startWarmupEngine() {
	log.Println("[Warmup Engine] Starting...")

	// Run every minute to check and send warmup emails
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	// Also run IMAP check every 5 minutes
	imapTicker := time.NewTicker(5 * time.Minute)
	defer imapTicker.Stop()

	for {
		select {
		case <-ticker.C:
			s.processWarmupEmails()
		case <-imapTicker.C:
			s.processIMAPInteractions()
		}
	}
}

func (s *Server) processWarmupEmails() {
	now := time.Now()
	currentHour := now.Hour()

	// Log active counts
	var activeSMTPs, activeSeeds, activeTemplates int
	s.db.QueryRow(`SELECT COUNT(*) FROM warmup_smtps WHERE status = 'active'`).Scan(&activeSMTPs)
	s.db.QueryRow(`SELECT COUNT(*) FROM warmup_seeds WHERE status = 'active'`).Scan(&activeSeeds)
	s.db.QueryRow(`SELECT COUNT(*) FROM warmup_templates WHERE active = true`).Scan(&activeTemplates)

	log.Printf("[Warmup Engine] Processing at %s (hour %d) - Active: %d SMTPs, %d Seeds, %d Templates",
		now.Format("15:04:05"), currentHour, activeSMTPs, activeSeeds, activeTemplates)

	if activeSeeds == 0 {
		log.Printf("[Warmup Engine] No active seeds available - cannot send warmup emails")
		return
	}
	if activeTemplates == 0 {
		log.Printf("[Warmup Engine] No active templates available - cannot send warmup emails")
		return
	}

	// Get active warmup SMTPs that should send now
	rows, err := s.db.Query(`
		SELECT w.id, w.smtp_id, w.start_date, w.min_emails_per_day, w.max_emails_per_day,
			   w.recipe_type, w.custom_schedule, w.reply_rate, w.start_hour, w.end_hour,
			   s.host, s.port, s.username, s.password, s.tls_mode
		FROM warmup_smtps w
		JOIN smtp_servers s ON w.smtp_id = s.id
		WHERE w.status = 'active'
		AND s.active = true
	`)
	if err != nil {
		log.Printf("[Warmup Engine] Error querying SMTPs: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var warmupID, smtpID, recipeType string
		var startDate time.Time
		var minEmails, maxEmails, replyRate, startHour, endHour int
		var customSchedule sql.NullString
		var host, username, password, tlsMode string
		var port int

		rows.Scan(&warmupID, &smtpID, &startDate, &minEmails, &maxEmails,
			&recipeType, &customSchedule, &replyRate, &startHour, &endHour,
			&host, &port, &username, &password, &tlsMode)

		// Calculate current day based on start date
		currentDay := int(now.Sub(startDate).Hours()/24) + 1
		if currentDay < 1 {
			currentDay = 1
		}

		// Update current_day in database for UI display
		s.db.Exec(`UPDATE warmup_smtps SET current_day = $1 WHERE id = $2`, currentDay, warmupID)

		log.Printf("[Warmup Engine] SMTP %s: Day %d, hours %d-%d, current hour %d", warmupID, currentDay, startHour, endHour, currentHour)

		// Check if within sending hours
		if currentHour < startHour || currentHour >= endHour {
			log.Printf("[Warmup Engine] SMTP %s: Outside sending hours, skipping", warmupID)
			continue
		}

		// Calculate today's limit
		todayLimit := s.calculateDailyLimit(recipeType, currentDay, minEmails, maxEmails, customSchedule.String)

		// Get how many already sent today
		var sentToday int
		s.db.QueryRow(`
			SELECT COUNT(*) FROM warmup_emails
			WHERE warmup_smtp_id = $1 AND DATE(sent_at) = $2
		`, warmupID, now.Format("2006-01-02")).Scan(&sentToday)

		log.Printf("[Warmup Engine] SMTP %s: Today's limit %d, sent so far %d", warmupID, todayLimit, sentToday)

		if sentToday >= todayLimit {
			log.Printf("[Warmup Engine] SMTP %s: Daily limit reached, skipping", warmupID)
			continue
		}

		// Calculate how many to send this minute (spread throughout the day)
		remaining := todayLimit - sentToday
		sendingHours := endHour - startHour
		if sendingHours <= 0 {
			sendingHours = 1
		}
		sendingMinutes := sendingHours * 60
		emailsPerMinute := remaining / sendingMinutes

		// Always send at least 1 if there's remaining quota (probabilistic for low volume)
		if emailsPerMinute < 1 && remaining > 0 {
			// Calculate probability: remaining emails / remaining minutes in window
			currentMinuteInWindow := (currentHour-startHour)*60 + time.Now().Minute()
			remainingMinutes := sendingMinutes - currentMinuteInWindow
			if remainingMinutes <= 0 {
				remainingMinutes = 1
			}
			probability := float64(remaining) / float64(remainingMinutes)
			if rand.Float64() < probability || remaining >= remainingMinutes {
				emailsPerMinute = 1
			}
		}

		if emailsPerMinute > remaining {
			emailsPerMinute = remaining
		}

		// Send warmup emails
		log.Printf("[Warmup Engine] SMTP %s: Sending %d emails this minute (remaining: %d)", warmupID, emailsPerMinute, remaining)
		for i := 0; i < emailsPerMinute; i++ {
			s.sendWarmupEmail(warmupID, smtpID, host, port, username, password, tlsMode, replyRate)
		}
	}
}

func (s *Server) calculateDailyLimit(recipeType string, currentDay, minEmails, maxEmails int, customSchedule string) int {
	switch recipeType {
	case "flat":
		return minEmails
	case "randomized":
		return minEmails + rand.Intn(maxEmails-minEmails+1)
	case "custom":
		if customSchedule != "" {
			var schedule []int
			if err := json.Unmarshal([]byte(customSchedule), &schedule); err == nil {
				if currentDay > 0 && currentDay <= len(schedule) {
					return schedule[currentDay-1]
				}
			}
		}
		fallthrough
	default: // progressive
		increment := float64(maxEmails-minEmails) / 45.0
		limit := minEmails + int(float64(currentDay-1)*increment)
		if limit > maxEmails {
			limit = maxEmails
		}
		return limit
	}
}

func (s *Server) sendWarmupEmail(warmupID, smtpID, host string, port int, username, password, tlsMode string, replyRate int) {
	// Get a random active seed
	var seedID, seedEmail string
	err := s.db.QueryRow(`
		SELECT id, email FROM warmup_seeds
		WHERE status = 'active'
		ORDER BY RANDOM()
		LIMIT 1
	`).Scan(&seedID, &seedEmail)

	if err != nil {
		log.Printf("[Warmup] No active seeds available")
		return
	}

	// Get a random sender from smtp_senders (or fall back to username)
	var senderEmail string
	var senderName sql.NullString
	err = s.db.QueryRow(`
		SELECT email, name FROM smtp_senders
		WHERE smtp_id = $1 AND active = true
		ORDER BY RANDOM()
		LIMIT 1
	`, smtpID).Scan(&senderEmail, &senderName)

	if err != nil || senderEmail == "" {
		// Fall back to SMTP username if no senders configured
		senderEmail = username
		log.Printf("[Warmup] No senders configured for SMTP, using username: %s", username)
	}

	// Format From address with name if available
	fromAddress := senderEmail
	if senderName.Valid && senderName.String != "" {
		fromAddress = fmt.Sprintf("%s <%s>", senderName.String, senderEmail)
	}

	// Get a random template
	var subject, body string
	err = s.db.QueryRow(`
		SELECT subject, body FROM warmup_templates
		WHERE active = true
		ORDER BY RANDOM()
		LIMIT 1
	`).Scan(&subject, &body)

	if err != nil {
		log.Printf("[Warmup] No templates available")
		return
	}

	// Add some randomization to subject
	subject = subject + " #" + fmt.Sprintf("%d", rand.Intn(9999))

	// Generate message ID
	messageID := fmt.Sprintf("<%s@warmup>", uuid.New().String())

	// Send email via SMTP
	err = s.sendSMTPEmail(host, port, username, password, tlsMode, fromAddress, seedEmail, subject, body, messageID)
	if err != nil {
		log.Printf("[Warmup] Failed to send from %s to %s: %v", senderEmail, seedEmail, err)
		return
	}

	// Record the email
	emailID := uuid.New().String()
	s.db.Exec(`
		INSERT INTO warmup_emails (id, warmup_smtp_id, seed_id, subject, message_id, status, sent_at)
		VALUES ($1, $2, $3, $4, $5, 'sent', NOW())
	`, emailID, warmupID, seedID, subject, messageID)

	// Update stats
	s.db.Exec(`
		UPDATE warmup_smtps SET total_sent = total_sent + 1, updated_at = NOW()
		WHERE id = $1
	`, warmupID)

	// Update daily stats
	s.updateWarmupDailyStats(warmupID, "sent")

	// Update sender stats
	s.db.Exec(`UPDATE smtp_senders SET total_sent = total_sent + 1 WHERE email = $1 AND smtp_id = $2`, senderEmail, smtpID)

	log.Printf("[Warmup] ✉️ Sent from %s to %s: %s", senderEmail, seedEmail, subject)
}

func (s *Server) sendSMTPEmail(host string, port int, username, password, tlsMode, from, to, subject, body, messageID string) error {
	// Extract email from "Name <email>" format if present
	fromEmail := from
	if strings.Contains(from, "<") && strings.Contains(from, ">") {
		start := strings.Index(from, "<") + 1
		end := strings.Index(from, ">")
		if start > 0 && end > start {
			fromEmail = from[start:end]
		}
	}

	msg := fmt.Sprintf("From: %s\r\n"+
		"To: %s\r\n"+
		"Subject: %s\r\n"+
		"Message-ID: %s\r\n"+
		"MIME-Version: 1.0\r\n"+
		"Content-Type: text/plain; charset=utf-8\r\n"+
		"\r\n"+
		"%s", from, to, subject, messageID, body)

	addr := fmt.Sprintf("%s:%d", host, port)

	// Handle different TLS modes like the campaign engine
	switch tlsMode {
	case "tls":
		return s.sendWithImplicitTLS(addr, host, username, password, fromEmail, to, []byte(msg))
	case "starttls":
		return s.sendWithSTARTTLS(addr, host, username, password, fromEmail, to, []byte(msg))
	default: // "none" or empty
		return s.sendPlainSMTP(addr, host, username, password, fromEmail, to, []byte(msg))
	}
}

// sendWithImplicitTLS sends email using implicit TLS (port 465)
func (s *Server) sendWithImplicitTLS(addr, host, username, password, from, to string, msg []byte) error {
	tlsConfig := &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: true,
	}

	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("TLS dial error: %v", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("SMTP client error: %v", err)
	}
	defer client.Close()

	if err := s.authenticateSMTP(client, host, username, password); err != nil {
		return err
	}

	return s.sendSMTPMessage(client, from, to, msg)
}

// sendWithSTARTTLS sends email using STARTTLS
func (s *Server) sendWithSTARTTLS(addr, host, username, password, from, to string, msg []byte) error {
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return fmt.Errorf("dial error: %v", err)
	}

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("SMTP client error: %v", err)
	}
	defer client.Close()

	if err := client.Hello("[127.0.0.1]"); err != nil {
		return fmt.Errorf("EHLO error: %v", err)
	}

	tlsConfig := &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: true,
	}

	if err := client.StartTLS(tlsConfig); err != nil {
		return fmt.Errorf("STARTTLS error: %v", err)
	}

	if err := s.authenticateSMTP(client, host, username, password); err != nil {
		return err
	}

	return s.sendSMTPMessage(client, from, to, msg)
}

// sendPlainSMTP sends email without TLS (for PowerMTA and similar)
func (s *Server) sendPlainSMTP(addr, host, username, password, from, to string, msg []byte) error {
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return fmt.Errorf("dial error: %v", err)
	}

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("SMTP client error: %v", err)
	}
	defer client.Close()

	if err := client.Hello("[127.0.0.1]"); err != nil {
		return fmt.Errorf("EHLO error: %v", err)
	}

	if err := s.authenticateSMTP(client, host, username, password); err != nil {
		return err
	}

	return s.sendSMTPMessage(client, from, to, msg)
}

// authenticateSMTP tries LOGIN auth first (works without TLS), then PLAIN
func (s *Server) authenticateSMTP(client *smtp.Client, host, username, password string) error {
	if username == "" || password == "" {
		return nil // No auth needed
	}

	// Try LOGIN auth first (doesn't require encrypted connection)
	if err := client.Auth(warmupLoginAuth(username, password)); err == nil {
		return nil
	}

	// Try PLAIN auth as fallback
	auth := smtp.PlainAuth("", username, password, host)
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("auth error (tried LOGIN, PLAIN): %v", err)
	}

	return nil
}

// sendSMTPMessage sends the email message after authentication
func (s *Server) sendSMTPMessage(client *smtp.Client, from, to string, msg []byte) error {
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("MAIL error: %v", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("RCPT error: %v", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("DATA error: %v", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("write error: %v", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close error: %v", err)
	}

	return client.Quit()
}

// warmupLoginAuth implements LOGIN authentication (works without TLS)
type warmupLoginAuthStruct struct {
	username, password string
}

func warmupLoginAuth(username, password string) smtp.Auth {
	return &warmupLoginAuthStruct{username, password}
}

func (a *warmupLoginAuthStruct) Start(server *smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", []byte{}, nil
}

func (a *warmupLoginAuthStruct) Next(fromServer []byte, more bool) ([]byte, error) {
	if more {
		switch string(fromServer) {
		case "Username:":
			return []byte(a.username), nil
		case "Password:":
			return []byte(a.password), nil
		default:
			return nil, fmt.Errorf("unknown from server: %s", string(fromServer))
		}
	}
	return nil, nil
}

func (s *Server) updateWarmupDailyStats(warmupID, statType string) {
	today := time.Now().Format("2006-01-02")

	// Try to update existing record
	result, _ := s.db.Exec(fmt.Sprintf(`
		UPDATE warmup_daily_stats SET %s = %s + 1
		WHERE warmup_smtp_id = $1 AND date = $2
	`, statType, statType), warmupID, today)

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		// Insert new record
		s.db.Exec(`
			INSERT INTO warmup_daily_stats (id, warmup_smtp_id, date, sent, inbox, spam, replies)
			VALUES ($1, $2, $3, 0, 0, 0, 0)
		`, uuid.New().String(), warmupID, today)

		// Update again
		s.db.Exec(fmt.Sprintf(`
			UPDATE warmup_daily_stats SET %s = %s + 1
			WHERE warmup_smtp_id = $1 AND date = $2
		`, statType, statType), warmupID, today)
	}
}

// ============================================
// IMAP INTERACTION PROCESSOR
// ============================================

func (s *Server) processIMAPInteractions() {
	// Count active seeds
	var activeSeeds int
	s.db.QueryRow(`SELECT COUNT(*) FROM warmup_seeds WHERE status = 'active'`).Scan(&activeSeeds)
	log.Printf("[Warmup IMAP] Checking %d active seed inboxes...", activeSeeds)

	if activeSeeds == 0 {
		log.Println("[Warmup IMAP] No active seeds to check")
		return
	}

	// Get all active seeds
	rows, err := s.db.Query(`
		SELECT id, email, password, imap_host, imap_port, use_tls
		FROM warmup_seeds WHERE status = 'active'
	`)
	if err != nil {
		log.Printf("[Warmup IMAP] Error: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var seedID, email, password, imapHost string
		var imapPort int
		var useTLS bool

		rows.Scan(&seedID, &email, &password, &imapHost, &imapPort, &useTLS)

		log.Printf("[Warmup IMAP] Processing inbox for %s", email)
		go s.processOneSeedInbox(seedID, email, password, imapHost, imapPort, useTLS)
	}
}

func (s *Server) processOneSeedInbox(seedID, email, password, imapHost string, imapPort int, useTLS bool) {
	addr := fmt.Sprintf("%s:%d", imapHost, imapPort)

	var c *client.Client
	var err error

	if useTLS || imapPort == 993 {
		c, err = client.DialTLS(addr, &tls.Config{ServerName: imapHost})
	} else {
		c, err = client.Dial(addr)
	}

	if err != nil {
		log.Printf("[Warmup IMAP] Failed to connect to %s: %v", email, err)
		s.db.Exec(`UPDATE warmup_seeds SET status = 'error', error_message = $1, last_check = NOW() WHERE id = $2`,
			err.Error(), seedID)
		return
	}
	defer c.Logout()

	if err := c.Login(email, password); err != nil {
		log.Printf("[Warmup IMAP] Login failed for %s: %v", email, err)
		s.db.Exec(`UPDATE warmup_seeds SET status = 'error', error_message = $1, last_check = NOW() WHERE id = $2`,
			err.Error(), seedID)
		return
	}

	// Update last check
	s.db.Exec(`UPDATE warmup_seeds SET last_check = NOW(), status = 'active', error_message = NULL WHERE id = $1`, seedID)

	// Check INBOX for warmup emails
	s.checkMailbox(c, seedID, "INBOX", false)

	// Check Spam/Junk folder
	spamFolders := []string{"[Gmail]/Spam", "Junk", "Spam", "Junk E-mail", "Bulk Mail"}
	for _, folder := range spamFolders {
		if s.checkMailbox(c, seedID, folder, true) {
			break
		}
	}

	// Random chance to reply (based on reply rate)
	s.maybeReplyToWarmupEmail(c, seedID, email, password, imapHost)
}

func (s *Server) checkMailbox(c *client.Client, seedID, mailbox string, isSpam bool) bool {
	mbox, err := c.Select(mailbox, false)
	if err != nil {
		return false
	}

	if mbox.Messages == 0 {
		return true
	}

	// Search for unseen messages
	criteria := imap.NewSearchCriteria()
	criteria.WithoutFlags = []string{imap.SeenFlag}

	ids, err := c.Search(criteria)
	if err != nil || len(ids) == 0 {
		return true
	}

	seqSet := new(imap.SeqSet)
	seqSet.AddNum(ids...)

	// Fetch messages
	messages := make(chan *imap.Message, 10)
	section := &imap.BodySectionName{Peek: true}
	go func() {
		c.Fetch(seqSet, []imap.FetchItem{imap.FetchEnvelope, section.FetchItem()}, messages)
	}()

	for msg := range messages {
		if msg == nil || msg.Envelope == nil {
			continue
		}

		// Check if this is a warmup email (by message ID)
		messageID := msg.Envelope.MessageId

		var warmupEmailID, warmupSMTPID string
		err := s.db.QueryRow(`
			SELECT id, warmup_smtp_id FROM warmup_emails
			WHERE message_id = $1 AND seed_id = $2
		`, "<"+messageID+">", seedID).Scan(&warmupEmailID, &warmupSMTPID)

		if err != nil {
			// Also try without angle brackets
			s.db.QueryRow(`
				SELECT id, warmup_smtp_id FROM warmup_emails
				WHERE message_id = $1 AND seed_id = $2
			`, messageID, seedID).Scan(&warmupEmailID, &warmupSMTPID)
		}

		if warmupEmailID == "" {
			continue
		}

		// Found a warmup email!
		log.Printf("[Warmup IMAP] Found warmup email in %s: %s", mailbox, msg.Envelope.Subject)

		// Mark as read (opens the email)
		item := imap.FormatFlagsOp(imap.AddFlags, true)
		flags := []interface{}{imap.SeenFlag}
		singleSeq := new(imap.SeqSet)
		singleSeq.AddNum(msg.SeqNum)
		c.Store(singleSeq, item, flags, nil)

		if isSpam {
			// Move from spam to inbox
			s.db.Exec(`UPDATE warmup_emails SET landed_in_spam = true WHERE id = $1`, warmupEmailID)
			s.db.Exec(`UPDATE warmup_smtps SET total_spam = total_spam + 1 WHERE id = $1`, warmupSMTPID)
			s.updateWarmupDailyStats(warmupSMTPID, "spam")

			// Try to move to inbox
			if err := c.Move(singleSeq, "INBOX"); err == nil {
				s.db.Exec(`UPDATE warmup_emails SET moved_to_inbox = true WHERE id = $1`, warmupEmailID)
				log.Printf("[Warmup IMAP] Moved email from spam to inbox")
			}
		} else {
			// Landed in inbox - good!
			s.db.Exec(`UPDATE warmup_emails SET status = 'opened', opened_at = NOW() WHERE id = $1`, warmupEmailID)
			s.db.Exec(`UPDATE warmup_smtps SET total_inbox = total_inbox + 1 WHERE id = $1`, warmupSMTPID)
			s.updateWarmupDailyStats(warmupSMTPID, "inbox")
		}
	}

	return true
}

func (s *Server) maybeReplyToWarmupEmail(c *client.Client, seedID, email, password, imapHost string) {
	// Get a warmup email that hasn't been replied to
	var warmupEmailID, warmupSMTPID, originalSubject, messageID string
	var smtpHost, smtpUsername, smtpPassword, smtpTLSMode string
	var smtpPort int

	err := s.db.QueryRow(`
		SELECT e.id, e.warmup_smtp_id, e.subject, e.message_id,
			   s.host, s.port, s.username, s.password, s.tls_mode
		FROM warmup_emails e
		JOIN warmup_smtps w ON e.warmup_smtp_id = w.id
		JOIN smtp_servers s ON w.smtp_id = s.id
		WHERE e.seed_id = $1 AND e.replied_at IS NULL AND e.status = 'opened'
		ORDER BY RANDOM()
		LIMIT 1
	`, seedID).Scan(&warmupEmailID, &warmupSMTPID, &originalSubject, &messageID,
		&smtpHost, &smtpPort, &smtpUsername, &smtpPassword, &smtpTLSMode)

	if err != nil {
		return
	}

	// Check reply rate
	var replyRate int
	s.db.QueryRow(`SELECT reply_rate FROM warmup_smtps WHERE id = $1`, warmupSMTPID).Scan(&replyRate)

	if rand.Intn(100) >= replyRate {
		return
	}

	// Send reply from seed to SMTP
	replySubject := "Re: " + originalSubject
	replyBody := getRandomReplyBody()

	// Get seed's SMTP settings
	var seedSMTPHost string
	var seedSMTPPort int
	s.db.QueryRow(`SELECT smtp_host, smtp_port FROM warmup_seeds WHERE id = $1`, seedID).Scan(&seedSMTPHost, &seedSMTPPort)

	if seedSMTPHost == "" {
		return
	}

	replyMessageID := fmt.Sprintf("<%s@warmup-reply>", uuid.New().String())
	// Seeds typically use STARTTLS - seed email is the From address
	err = s.sendSMTPEmail(seedSMTPHost, seedSMTPPort, email, password, "starttls", email, smtpUsername, replySubject, replyBody, replyMessageID)

	if err != nil {
		log.Printf("[Warmup Reply] Failed to send reply: %v", err)
		return
	}

	// Update stats
	s.db.Exec(`UPDATE warmup_emails SET replied_at = NOW(), status = 'replied' WHERE id = $1`, warmupEmailID)
	s.db.Exec(`UPDATE warmup_smtps SET total_replies = total_replies + 1 WHERE id = $1`, warmupSMTPID)
	s.updateWarmupDailyStats(warmupSMTPID, "replies")

	log.Printf("[Warmup Reply] ↩️ Replied from %s to warmup email", email)
}

func getRandomReplyBody() string {
	replies := []string{
		"Thanks for reaching out! I'll get back to you soon.",
		"Got it, thanks for the information.",
		"Thank you for your message. I'll review and respond shortly.",
		"Received, thanks!",
		"Thanks for the update. I appreciate it.",
		"Perfect, thank you for letting me know.",
		"Great, I'll take a look at this.",
		"Thanks! I'll be in touch.",
		"Noted. Thanks for sending this over.",
		"Thank you, I'll follow up on this soon.",
	}
	return replies[rand.Intn(len(replies))]
}
