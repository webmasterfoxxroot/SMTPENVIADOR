package api

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// Web Warmup Account
type WebWarmupAccount struct {
	ID           string     `json:"id"`
	UserID       string     `json:"user_id"`
	Email        string     `json:"email"`
	Password     string     `json:"password,omitempty"`
	RefreshToken string     `json:"refresh_token,omitempty"` // OAuth2 refresh token for Graph API
	ClientID     string     `json:"client_id,omitempty"`     // OAuth2 client ID
	Provider     string     `json:"provider"`                // outlook, gmail
	Status       string     `json:"status"`                  // active, paused, error
	EmailsSent   int        `json:"emails_sent"`
	LastActivity *time.Time `json:"last_activity"`
	ErrorMessage string     `json:"error_message,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
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
			refresh_token TEXT,
			client_id VARCHAR(255),
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

	// Add columns if they don't exist (for existing tables)
	s.db.Exec(`ALTER TABLE web_warmup_accounts ADD COLUMN IF NOT EXISTS refresh_token TEXT`)
	s.db.Exec(`ALTER TABLE web_warmup_accounts ADD COLUMN IF NOT EXISTS client_id VARCHAR(255)`)

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
		Email        string `json:"email"`
		Password     string `json:"password"`
		RefreshToken string `json:"refresh_token"`
		ClientID     string `json:"client_id"`
		Provider     string `json:"provider"`
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
		INSERT INTO web_warmup_accounts (id, user_id, email, password, refresh_token, client_id, provider, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'active')
	`, id, userID, req.Email, req.Password, req.RefreshToken, req.ClientID, req.Provider)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"id": id, "message": "Account added"})
}

// Import web warmup accounts (bulk import with format: email TAB password TAB token TAB client_id)
func (s *Server) importWebWarmupAccounts(c *fiber.Ctx) error {
	userID := getUserID(c)

	var req struct {
		Accounts string `json:"accounts"` // Tab-separated accounts, one per line
		Provider string `json:"provider"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.Provider == "" {
		req.Provider = "outlook"
	}

	lines := strings.Split(strings.TrimSpace(req.Accounts), "\n")
	imported := 0
	errors := []string{}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse format: email:password:token:client_id (colon) or email TAB password TAB token TAB client_id (tab)
		var parts []string
		if strings.Contains(line, "\t") {
			parts = strings.Split(line, "\t")
		} else {
			parts = strings.Split(line, ":")
		}
		if len(parts) < 2 {
			errors = append(errors, "Invalid format: "+line[:min(30, len(line))])
			continue
		}

		email := strings.TrimSpace(parts[0])
		password := strings.TrimSpace(parts[1])
		refreshToken := ""
		clientID := ""

		if len(parts) >= 3 {
			refreshToken = strings.TrimSpace(parts[2])
		}
		if len(parts) >= 4 {
			clientID = strings.TrimSpace(parts[3])
		}

		if email == "" || password == "" {
			errors = append(errors, "Missing email or password")
			continue
		}

		id := uuid.New().String()
		_, err := s.db.Exec(`
			INSERT INTO web_warmup_accounts (id, user_id, email, password, refresh_token, client_id, provider, status)
			VALUES ($1, $2, $3, $4, $5, $6, $7, 'active')
		`, id, userID, email, password, refreshToken, clientID, req.Provider)
		if err != nil {
			errors = append(errors, fmt.Sprintf("Error importing %s: %v", email, err))
			continue
		}
		imported++
	}

	return c.JSON(fiber.Map{
		"imported": imported,
		"errors":   errors,
		"message":  fmt.Sprintf("Imported %d accounts", imported),
	})
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

	// Get account details (including refresh_token and client_id)
	var email, password, provider string
	var refreshToken, clientID *string
	err := s.db.QueryRow(`
		SELECT email, password, provider, refresh_token, client_id FROM web_warmup_accounts
		WHERE id = $1 AND user_id = $2
	`, accountID, userID).Scan(&email, &password, &provider, &refreshToken, &clientID)
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

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Use Graph API if refresh_token and client_id are available
	if refreshToken != nil && clientID != nil && *refreshToken != "" && *clientID != "" {
		log.Printf("[Web Warmup] Testing account %s using Graph API", email)
		result, err := s.testGraphAPIConnection(ctx, email, *refreshToken, *clientID, &settings)
		if err != nil {
			// Update account status to error
			s.db.Exec(`UPDATE web_warmup_accounts SET status = 'error', error_message = $1 WHERE id = $2`,
				err.Error(), accountID)
			return c.JSON(fiber.Map{"success": false, "error": err.Error(), "method": "graph_api"})
		}

		// Update account status to active
		s.db.Exec(`UPDATE web_warmup_accounts SET status = 'active', error_message = NULL, last_activity = NOW() WHERE id = $1`, accountID)
		return c.JSON(fiber.Map{"success": true, "ip": result.IP, "message": result.Message, "method": "graph_api"})
	}

	// Fall back to browser automation if no token
	log.Printf("[Web Warmup] Testing account %s using browser automation (no token)", email)
	result, err := s.testOutlookWebLogin(ctx, email, password, &settings)
	if err != nil {
		// Update account status to error
		s.db.Exec(`UPDATE web_warmup_accounts SET status = 'error', error_message = $1 WHERE id = $2`,
			err.Error(), accountID)
		return c.JSON(fiber.Map{"success": false, "error": err.Error(), "method": "browser"})
	}

	// Update account status to active
	s.db.Exec(`UPDATE web_warmup_accounts SET status = 'active', error_message = NULL, last_activity = NOW() WHERE id = $1`, accountID)

	return c.JSON(fiber.Map{"success": true, "ip": result.IP, "message": "Login successful", "method": "browser"})
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

// Send test email from web warmup account
func (s *Server) sendTestWebWarmupEmail(c *fiber.Ctx) error {
	userID := getUserID(c)
	accountID := c.Params("id")

	var req struct {
		ToEmail string `json:"to_email"`
		Subject string `json:"subject"`
		Body    string `json:"body"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.ToEmail == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Recipient email required"})
	}

	if req.Subject == "" {
		req.Subject = "Web Warmup Test Email"
	}
	if req.Body == "" {
		req.Body = "<p>This is a test email from Web Warmup.</p><p>If you received this, your account is working correctly.</p>"
	}

	// Get account details (including refresh_token and client_id)
	var email, password string
	var refreshToken, clientID *string
	err := s.db.QueryRow(`
		SELECT email, password, refresh_token, client_id FROM web_warmup_accounts
		WHERE id = $1 AND user_id = $2
	`, accountID, userID).Scan(&email, &password, &refreshToken, &clientID)
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

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Use Graph API if refresh_token and client_id are available
	if refreshToken != nil && clientID != nil && *refreshToken != "" && *clientID != "" {
		log.Printf("[Web Warmup] Sending test email from %s to %s using Graph API", email, req.ToEmail)
		result, err := s.sendGraphAPIEmail(ctx, email, *refreshToken, *clientID, req.ToEmail, req.Subject, req.Body, &settings)
		if err != nil {
			return c.JSON(fiber.Map{"success": false, "error": err.Error(), "method": "graph_api"})
		}

		// Record the email
		s.db.Exec(`INSERT INTO web_warmup_emails (account_id, to_email, subject, status, proxy_ip) VALUES ($1, $2, $3, 'sent', $4)`,
			accountID, req.ToEmail, req.Subject, result.IP)
		s.db.Exec(`UPDATE web_warmup_accounts SET emails_sent = emails_sent + 1, last_activity = NOW() WHERE id = $1`, accountID)

		return c.JSON(fiber.Map{"success": true, "ip": result.IP, "message": "Email sent via Graph API", "method": "graph_api"})
	}

	// Fall back to browser automation if no token
	log.Printf("[Web Warmup] Sending test email from %s to %s using browser automation", email, req.ToEmail)
	result, err := s.sendOutlookWebEmail(ctx, email, password, req.ToEmail, req.Subject, req.Body, &settings)
	if err != nil {
		return c.JSON(fiber.Map{"success": false, "error": err.Error(), "method": "browser"})
	}

	// Record the email
	s.db.Exec(`INSERT INTO web_warmup_emails (account_id, to_email, subject, status, proxy_ip) VALUES ($1, $2, $3, 'sent', $4)`,
		accountID, req.ToEmail, req.Subject, result.IP)
	s.db.Exec(`UPDATE web_warmup_accounts SET emails_sent = emails_sent + 1, last_activity = NOW() WHERE id = $1`, accountID)

	return c.JSON(fiber.Map{"success": true, "ip": result.IP, "message": "Email sent via browser", "method": "browser"})
}

// Register web warmup routes
func (s *Server) registerWebWarmupRoutes(api fiber.Router) {
	// Initialize tables
	s.initWebWarmupTables()

	webWarmup := api.Group("/web-warmup")

	// Accounts
	webWarmup.Get("/accounts", s.listWebWarmupAccounts)
	webWarmup.Post("/accounts", s.addWebWarmupAccount)
	webWarmup.Post("/accounts/import", s.importWebWarmupAccounts) // Bulk import with token/client_id
	webWarmup.Put("/accounts/:id", s.updateWebWarmupAccount)
	webWarmup.Delete("/accounts/:id", s.deleteWebWarmupAccount)
	webWarmup.Post("/accounts/:id/test", s.testWebWarmupAccount)
	webWarmup.Post("/accounts/:id/send-test-email", s.sendTestWebWarmupEmail) // Send test email

	// Stats & Settings
	webWarmup.Get("/stats", s.getWebWarmupStats)
	webWarmup.Get("/settings", s.getWebWarmupSettings)
	webWarmup.Put("/settings", s.updateWebWarmupSettings)
}
