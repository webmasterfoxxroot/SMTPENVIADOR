package api

import (
	"context"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// Web Warmup Account
type WebWarmupAccount struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	Email        string    `json:"email"`
	Password     string    `json:"password,omitempty"`
	Provider     string    `json:"provider"` // outlook, gmail
	Status       string    `json:"status"`   // active, paused, error
	EmailsSent   int       `json:"emails_sent"`
	LastActivity *time.Time `json:"last_activity"`
	ErrorMessage string    `json:"error_message,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// Web Warmup Settings
type WebWarmupSettings struct {
	Enabled           bool   `json:"enabled"`
	EmailsPerDay      int    `json:"emails_per_day"`
	DelayBetweenEmails int    `json:"delay_between_emails"`
	UseProxy          bool   `json:"use_proxy"`
	ProxyHost         string `json:"proxy_host"`
	ProxyPort         string `json:"proxy_port"`
	ProxyUser         string `json:"proxy_user"`
	ProxyPass         string `json:"proxy_pass"`
}

// Initialize web warmup tables
func (s *Server) initWebWarmupTables() {
	// Create web_warmup_accounts table
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS web_warmup_accounts (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			email VARCHAR(255) NOT NULL,
			password VARCHAR(255) NOT NULL,
			provider VARCHAR(50) DEFAULT 'outlook',
			status VARCHAR(50) DEFAULT 'active',
			emails_sent INT DEFAULT 0,
			last_activity TIMESTAMP,
			error_message TEXT,
			created_at TIMESTAMP DEFAULT NOW()
		)
	`)
	if err != nil {
		log.Printf("[Web Warmup] Error creating web_warmup_accounts table: %v", err)
	}

	// Create web_warmup_settings table
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS web_warmup_settings (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			enabled BOOLEAN DEFAULT false,
			emails_per_day INT DEFAULT 10,
			delay_between_emails INT DEFAULT 60,
			use_proxy BOOLEAN DEFAULT false,
			proxy_host VARCHAR(255),
			proxy_port VARCHAR(50),
			proxy_user VARCHAR(255),
			proxy_pass VARCHAR(255),
			created_at TIMESTAMP DEFAULT NOW(),
			updated_at TIMESTAMP DEFAULT NOW(),
			UNIQUE(user_id)
		)
	`)
	if err != nil {
		log.Printf("[Web Warmup] Error creating web_warmup_settings table: %v", err)
	}

	// Create web_warmup_emails table for tracking sent emails
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS web_warmup_emails (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			account_id UUID NOT NULL REFERENCES web_warmup_accounts(id) ON DELETE CASCADE,
			to_email VARCHAR(255) NOT NULL,
			subject VARCHAR(500),
			status VARCHAR(50) DEFAULT 'sent',
			proxy_ip VARCHAR(50),
			sent_at TIMESTAMP DEFAULT NOW()
		)
	`)
	if err != nil {
		log.Printf("[Web Warmup] Error creating web_warmup_emails table: %v", err)
	}

	log.Println("[Web Warmup] Tables initialized")
}

// List web warmup accounts
func (s *Server) listWebWarmupAccounts(c *fiber.Ctx) error {
	userID := getUserID(c)

	rows, err := s.db.Query(`
		SELECT id, email, provider, status, emails_sent, last_activity, error_message, created_at
		FROM web_warmup_accounts
		WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	var accounts []WebWarmupAccount
	for rows.Next() {
		var acc WebWarmupAccount
		var lastActivity *time.Time
		var errorMsg *string
		rows.Scan(&acc.ID, &acc.Email, &acc.Provider, &acc.Status, &acc.EmailsSent, &lastActivity, &errorMsg, &acc.CreatedAt)
		acc.LastActivity = lastActivity
		if errorMsg != nil {
			acc.ErrorMessage = *errorMsg
		}
		accounts = append(accounts, acc)
	}

	if accounts == nil {
		accounts = []WebWarmupAccount{}
	}

	return c.JSON(accounts)
}

// Add web warmup account
func (s *Server) addWebWarmupAccount(c *fiber.Ctx) error {
	userID := getUserID(c)

	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Provider string `json:"provider"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.Email == "" || req.Password == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Email and password required"})
	}

	if req.Provider == "" {
		req.Provider = "outlook"
	}

	id := uuid.New().String()
	_, err := s.db.Exec(`
		INSERT INTO web_warmup_accounts (id, user_id, email, password, provider, status)
		VALUES ($1, $2, $3, $4, $5, 'active')
	`, id, userID, req.Email, req.Password, req.Provider)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"id": id, "message": "Account added"})
}

// Update web warmup account
func (s *Server) updateWebWarmupAccount(c *fiber.Ctx) error {
	userID := getUserID(c)
	accountID := c.Params("id")

	var req struct {
		Status string `json:"status"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	_, err := s.db.Exec(`
		UPDATE web_warmup_accounts
		SET status = $1
		WHERE id = $2 AND user_id = $3
	`, req.Status, accountID, userID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Account updated"})
}

// Delete web warmup account
func (s *Server) deleteWebWarmupAccount(c *fiber.Ctx) error {
	userID := getUserID(c)
	accountID := c.Params("id")

	_, err := s.db.Exec(`
		DELETE FROM web_warmup_accounts
		WHERE id = $1 AND user_id = $2
	`, accountID, userID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Account deleted"})
}

// Test web warmup account login
func (s *Server) testWebWarmupAccount(c *fiber.Ctx) error {
	userID := getUserID(c)
	accountID := c.Params("id")

	// Get account details
	var email, password, provider string
	err := s.db.QueryRow(`
		SELECT email, password, provider FROM web_warmup_accounts
		WHERE id = $1 AND user_id = $2
	`, accountID, userID).Scan(&email, &password, &provider)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Account not found"})
	}

	// Get proxy settings
	var settings WebWarmupSettings
	s.db.QueryRow(`
		SELECT COALESCE(use_proxy, false), COALESCE(proxy_host, ''), COALESCE(proxy_port, ''),
		       COALESCE(proxy_user, ''), COALESCE(proxy_pass, '')
		FROM web_warmup_settings WHERE user_id = $1
	`, userID).Scan(&settings.UseProxy, &settings.ProxyHost, &settings.ProxyPort, &settings.ProxyUser, &settings.ProxyPass)

	// Test login using browser automation
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := s.testOutlookWebLogin(ctx, email, password, &settings)
	if err != nil {
		// Update account status to error
		s.db.Exec(`UPDATE web_warmup_accounts SET status = 'error', error_message = $1 WHERE id = $2`,
			err.Error(), accountID)
		return c.JSON(fiber.Map{"success": false, "error": err.Error()})
	}

	// Update account status to active
	s.db.Exec(`UPDATE web_warmup_accounts SET status = 'active', error_message = NULL, last_activity = NOW() WHERE id = $1`, accountID)

	return c.JSON(fiber.Map{"success": true, "ip": result.IP, "message": "Login successful"})
}

// Get web warmup stats
func (s *Server) getWebWarmupStats(c *fiber.Ctx) error {
	userID := getUserID(c)

	var totalAccounts, activeAccounts int
	s.db.QueryRow(`SELECT COUNT(*) FROM web_warmup_accounts WHERE user_id = $1`, userID).Scan(&totalAccounts)
	s.db.QueryRow(`SELECT COUNT(*) FROM web_warmup_accounts WHERE user_id = $1 AND status = 'active'`, userID).Scan(&activeAccounts)

	var emailsToday int
	s.db.QueryRow(`
		SELECT COUNT(*) FROM web_warmup_emails e
		JOIN web_warmup_accounts a ON e.account_id = a.id
		WHERE a.user_id = $1 AND DATE(e.sent_at) = CURRENT_DATE
	`, userID).Scan(&emailsToday)

	var successRate float64 = 100
	var totalEmails, successEmails int
	s.db.QueryRow(`
		SELECT COUNT(*), COUNT(CASE WHEN status = 'sent' THEN 1 END) FROM web_warmup_emails e
		JOIN web_warmup_accounts a ON e.account_id = a.id
		WHERE a.user_id = $1
	`, userID).Scan(&totalEmails, &successEmails)
	if totalEmails > 0 {
		successRate = float64(successEmails) / float64(totalEmails) * 100
	}

	return c.JSON(fiber.Map{
		"total_accounts":  totalAccounts,
		"active_accounts": activeAccounts,
		"emails_today":    emailsToday,
		"success_rate":    int(successRate),
	})
}

// Get web warmup settings
func (s *Server) getWebWarmupSettings(c *fiber.Ctx) error {
	userID := getUserID(c)

	var settings WebWarmupSettings
	err := s.db.QueryRow(`
		SELECT enabled, emails_per_day, delay_between_emails, use_proxy,
		       COALESCE(proxy_host, ''), COALESCE(proxy_port, ''),
		       COALESCE(proxy_user, ''), COALESCE(proxy_pass, '')
		FROM web_warmup_settings WHERE user_id = $1
	`, userID).Scan(&settings.Enabled, &settings.EmailsPerDay, &settings.DelayBetweenEmails,
		&settings.UseProxy, &settings.ProxyHost, &settings.ProxyPort, &settings.ProxyUser, &settings.ProxyPass)

	if err != nil {
		// Return default settings
		settings = WebWarmupSettings{
			Enabled:           false,
			EmailsPerDay:      10,
			DelayBetweenEmails: 60,
			UseProxy:          false,
		}
	}

	return c.JSON(settings)
}

// Update web warmup settings
func (s *Server) updateWebWarmupSettings(c *fiber.Ctx) error {
	userID := getUserID(c)

	var settings WebWarmupSettings
	if err := c.BodyParser(&settings); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	_, err := s.db.Exec(`
		INSERT INTO web_warmup_settings (user_id, enabled, emails_per_day, delay_between_emails,
		                                  use_proxy, proxy_host, proxy_port, proxy_user, proxy_pass)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (user_id) DO UPDATE SET
			enabled = $2, emails_per_day = $3, delay_between_emails = $4,
			use_proxy = $5, proxy_host = $6, proxy_port = $7, proxy_user = $8, proxy_pass = $9,
			updated_at = NOW()
	`, userID, settings.Enabled, settings.EmailsPerDay, settings.DelayBetweenEmails,
		settings.UseProxy, settings.ProxyHost, settings.ProxyPort, settings.ProxyUser, settings.ProxyPass)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Settings updated"})
}

// Register web warmup routes
func (s *Server) registerWebWarmupRoutes(api fiber.Router) {
	// Initialize tables
	s.initWebWarmupTables()

	webWarmup := api.Group("/web-warmup")

	// Accounts
	webWarmup.Get("/accounts", s.listWebWarmupAccounts)
	webWarmup.Post("/accounts", s.addWebWarmupAccount)
	webWarmup.Put("/accounts/:id", s.updateWebWarmupAccount)
	webWarmup.Delete("/accounts/:id", s.deleteWebWarmupAccount)
	webWarmup.Post("/accounts/:id/test", s.testWebWarmupAccount)

	// Stats & Settings
	webWarmup.Get("/stats", s.getWebWarmupStats)
	webWarmup.Get("/settings", s.getWebWarmupSettings)
	webWarmup.Put("/settings", s.updateWebWarmupSettings)
}
