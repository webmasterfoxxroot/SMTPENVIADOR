package api

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"net"
	"net/smtp"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type SMTPRequest struct {
	Name           string `json:"name"`
	Host           string `json:"host"`
	Port           int    `json:"port"`
	Username       string `json:"username"`
	Password       string `json:"password"`
	TLSMode        string `json:"tls_mode"` // none, starttls, tls
	MaxPerMinute   int    `json:"max_per_minute"`
	MaxPerHour     int    `json:"max_per_hour"`
	MaxConnections int    `json:"max_connections"`
	Active         bool   `json:"active"`
}

// listSMTPs returns all SMTP servers
func (s *Server) listSMTPs(c *fiber.Ctx) error {
	rows, err := s.db.Query(`
		SELECT id, name, host, port, username, tls_mode,
		       max_per_minute, max_per_hour, max_connections,
		       active, status, last_check, total_sent, total_failed,
		       created_at, updated_at
		FROM smtp_servers
		ORDER BY created_at DESC
	`)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch SMTP servers"})
	}
	defer rows.Close()

	var servers []fiber.Map
	for rows.Next() {
		var id, name, host, username, status, tlsMode string
		var port, maxPerMinute, maxPerHour, maxConnections int
		var active bool
		var totalSent, totalFailed int64
		var lastCheck, createdAt, updatedAt *time.Time

		err := rows.Scan(&id, &name, &host, &port, &username, &tlsMode,
			&maxPerMinute, &maxPerHour, &maxConnections,
			&active, &status, &lastCheck, &totalSent, &totalFailed,
			&createdAt, &updatedAt)
		if err != nil {
			continue
		}

		servers = append(servers, fiber.Map{
			"id":              id,
			"name":            name,
			"host":            host,
			"port":            port,
			"username":        username,
			"tls_mode":        tlsMode,
			"max_per_minute":  maxPerMinute,
			"max_per_hour":    maxPerHour,
			"max_connections": maxConnections,
			"active":          active,
			"status":          status,
			"last_check":      lastCheck,
			"total_sent":      totalSent,
			"total_failed":    totalFailed,
			"created_at":      createdAt,
			"updated_at":      updatedAt,
		})
	}

	return c.JSON(fiber.Map{
		"data":  servers,
		"total": len(servers),
	})
}

// createSMTP creates a new SMTP server
func (s *Server) createSMTP(c *fiber.Ctx) error {
	var req SMTPRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// Validate
	if req.Name == "" || req.Host == "" || req.Username == "" || req.Password == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Missing required fields"})
	}

	if req.Port == 0 {
		req.Port = 587
	}
	if req.TLSMode == "" {
		req.TLSMode = "starttls"
	}
	// Validate TLS mode
	if req.TLSMode != "none" && req.TLSMode != "starttls" && req.TLSMode != "tls" {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tls_mode. Use: none, starttls, or tls"})
	}
	if req.MaxPerMinute == 0 {
		req.MaxPerMinute = 1000
	}
	if req.MaxPerHour == 0 {
		req.MaxPerHour = 50000
	}
	if req.MaxConnections == 0 {
		req.MaxConnections = 5
	}

	id := uuid.New().String()

	_, err := s.db.Exec(`
		INSERT INTO smtp_servers (id, name, host, port, username, password, tls_mode,
		                          max_per_minute, max_per_hour, max_connections, active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, id, req.Name, req.Host, req.Port, req.Username, req.Password, req.TLSMode,
		req.MaxPerMinute, req.MaxPerHour, req.MaxConnections, req.Active)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create SMTP server"})
	}

	// Refresh engine SMTP pool
	s.engine.RefreshSMTPs()

	return c.Status(201).JSON(fiber.Map{
		"message": "SMTP server created",
		"id":      id,
	})
}

// getSMTP returns a single SMTP server
func (s *Server) getSMTP(c *fiber.Ctx) error {
	id := c.Params("id")

	var name, host, username, status, tlsMode string
	var port, maxPerMinute, maxPerHour, maxConnections int
	var active bool
	var totalSent, totalFailed int64
	var lastCheck, createdAt, updatedAt *time.Time

	err := s.db.QueryRow(`
		SELECT name, host, port, username, tls_mode,
		       max_per_minute, max_per_hour, max_connections,
		       active, status, last_check, total_sent, total_failed,
		       created_at, updated_at
		FROM smtp_servers WHERE id = $1
	`, id).Scan(&name, &host, &port, &username, &tlsMode,
		&maxPerMinute, &maxPerHour, &maxConnections,
		&active, &status, &lastCheck, &totalSent, &totalFailed,
		&createdAt, &updatedAt)

	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "SMTP server not found"})
	}

	return c.JSON(fiber.Map{
		"id":              id,
		"name":            name,
		"host":            host,
		"port":            port,
		"username":        username,
		"tls_mode":        tlsMode,
		"max_per_minute":  maxPerMinute,
		"max_per_hour":    maxPerHour,
		"max_connections": maxConnections,
		"active":          active,
		"status":          status,
		"last_check":      lastCheck,
		"total_sent":      totalSent,
		"total_failed":    totalFailed,
		"created_at":      createdAt,
		"updated_at":      updatedAt,
	})
}

// updateSMTP updates an SMTP server
func (s *Server) updateSMTP(c *fiber.Ctx) error {
	id := c.Params("id")

	var req SMTPRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// Validate TLS mode if provided
	if req.TLSMode != "" && req.TLSMode != "none" && req.TLSMode != "starttls" && req.TLSMode != "tls" {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tls_mode. Use: none, starttls, or tls"})
	}

	query := `
		UPDATE smtp_servers SET
			name = $1, host = $2, port = $3, username = $4,
			tls_mode = $5, max_per_minute = $6, max_per_hour = $7,
			max_connections = $8, active = $9
		WHERE id = $10
	`
	args := []interface{}{req.Name, req.Host, req.Port, req.Username,
		req.TLSMode, req.MaxPerMinute, req.MaxPerHour, req.MaxConnections, req.Active, id}

	// If password provided, update it too
	if req.Password != "" {
		query = `
			UPDATE smtp_servers SET
				name = $1, host = $2, port = $3, username = $4, password = $5,
				tls_mode = $6, max_per_minute = $7, max_per_hour = $8,
				max_connections = $9, active = $10
			WHERE id = $11
		`
		args = []interface{}{req.Name, req.Host, req.Port, req.Username, req.Password,
			req.TLSMode, req.MaxPerMinute, req.MaxPerHour, req.MaxConnections, req.Active, id}
	}

	result, err := s.db.Exec(query, args...)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update SMTP server"})
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "SMTP server not found"})
	}

	// Refresh engine SMTP pool
	s.engine.RefreshSMTPs()

	return c.JSON(fiber.Map{"message": "SMTP server updated"})
}

// deleteSMTP deletes an SMTP server
func (s *Server) deleteSMTP(c *fiber.Ctx) error {
	id := c.Params("id")

	result, err := s.db.Exec(`DELETE FROM smtp_servers WHERE id = $1`, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete SMTP server"})
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "SMTP server not found"})
	}

	// Refresh engine SMTP pool
	s.engine.RefreshSMTPs()

	return c.JSON(fiber.Map{"message": "SMTP server deleted"})
}

// testSMTP tests an SMTP connection
func (s *Server) testSMTP(c *fiber.Ctx) error {
	id := c.Params("id")

	var host, username, password, tlsMode string
	var port int

	err := s.db.QueryRow(`
		SELECT host, port, username, password, tls_mode FROM smtp_servers WHERE id = $1
	`, id).Scan(&host, &port, &username, &password, &tlsMode)

	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "SMTP server not found"})
	}

	addr := fmt.Sprintf("%s:%d", host, port)
	var client *smtp.Client

	// Handle different TLS modes
	switch tlsMode {
	case "tls":
		// Implicit TLS connection (like port 465)
		tlsConfig := &tls.Config{
			ServerName:         host,
			InsecureSkipVerify: true, // Allow self-signed certs
		}
		conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 10 * time.Second}, "tcp", addr, tlsConfig)
		if err != nil {
			s.db.Exec(`UPDATE smtp_servers SET status = 'offline', last_check = NOW() WHERE id = $1`, id)
			return c.Status(400).JSON(fiber.Map{
				"error":   "TLS connection failed",
				"details": err.Error(),
			})
		}
		defer conn.Close()

		client, err = smtp.NewClient(conn, host)
		if err != nil {
			s.db.Exec(`UPDATE smtp_servers SET status = 'error', last_check = NOW() WHERE id = $1`, id)
			return c.Status(400).JSON(fiber.Map{
				"error":   "SMTP client failed",
				"details": err.Error(),
			})
		}

	case "starttls":
		// Regular connection with STARTTLS
		conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
		if err != nil {
			s.db.Exec(`UPDATE smtp_servers SET status = 'offline', last_check = NOW() WHERE id = $1`, id)
			return c.Status(400).JSON(fiber.Map{
				"error":   "Connection failed",
				"details": err.Error(),
			})
		}
		defer conn.Close()

		client, err = smtp.NewClient(conn, host)
		if err != nil {
			s.db.Exec(`UPDATE smtp_servers SET status = 'error', last_check = NOW() WHERE id = $1`, id)
			return c.Status(400).JSON(fiber.Map{
				"error":   "SMTP client failed",
				"details": err.Error(),
			})
		}

		// Say hello first
		if err := client.Hello("localhost"); err != nil {
			s.db.Exec(`UPDATE smtp_servers SET status = 'error', last_check = NOW() WHERE id = $1`, id)
			return c.Status(400).JSON(fiber.Map{
				"error":   "SMTP HELO failed",
				"details": err.Error(),
			})
		}

		// Use STARTTLS
		tlsConfig := &tls.Config{
			ServerName:         host,
			InsecureSkipVerify: true, // Allow self-signed certs
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			s.db.Exec(`UPDATE smtp_servers SET status = 'error', last_check = NOW() WHERE id = $1`, id)
			return c.Status(400).JSON(fiber.Map{
				"error":   "STARTTLS failed",
				"details": err.Error(),
			})
		}

	default: // "none" or empty
		// Plain connection without TLS
		conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
		if err != nil {
			s.db.Exec(`UPDATE smtp_servers SET status = 'offline', last_check = NOW() WHERE id = $1`, id)
			return c.Status(400).JSON(fiber.Map{
				"error":   "Connection failed",
				"details": err.Error(),
			})
		}
		defer conn.Close()

		client, err = smtp.NewClient(conn, host)
		if err != nil {
			s.db.Exec(`UPDATE smtp_servers SET status = 'error', last_check = NOW() WHERE id = $1`, id)
			return c.Status(400).JSON(fiber.Map{
				"error":   "SMTP client failed",
				"details": err.Error(),
			})
		}

		// Say hello
		if err := client.Hello("localhost"); err != nil {
			s.db.Exec(`UPDATE smtp_servers SET status = 'error', last_check = NOW() WHERE id = $1`, id)
			return c.Status(400).JSON(fiber.Map{
				"error":   "SMTP HELO failed",
				"details": err.Error(),
			})
		}
	}
	defer client.Close()

	// Test SMTP AUTH - try CRAM-MD5 first, then LOGIN, then PLAIN
	authErr := client.Auth(CRAMMD5Auth(username, password))
	if authErr != nil {
		authErr = client.Auth(LoginAuth(username, password))
	}
	if authErr != nil {
		auth := smtp.PlainAuth("", username, password, host)
		authErr = client.Auth(auth)
	}
	if authErr != nil {
		s.db.Exec(`UPDATE smtp_servers SET status = 'error', last_check = NOW() WHERE id = $1`, id)
		return c.Status(400).JSON(fiber.Map{
			"error":   "Authentication failed",
			"details": authErr.Error(),
		})
	}

	// Update status to online
	s.db.Exec(`UPDATE smtp_servers SET status = 'online', last_check = NOW() WHERE id = $1`, id)

	// Refresh engine SMTP pool to update in-memory status
	s.engine.RefreshSMTPs()

	return c.JSON(fiber.Map{
		"message": "SMTP connection successful",
		"status":  "online",
	})
}

// CRAMMD5Auth implements CRAM-MD5 authentication
type cramMD5Auth struct {
	username, password string
}

func CRAMMD5Auth(username, password string) smtp.Auth {
	return &cramMD5Auth{username, password}
}

func (a *cramMD5Auth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	return "CRAM-MD5", nil, nil
}

func (a *cramMD5Auth) Next(fromServer []byte, more bool) ([]byte, error) {
	if more {
		h := hmac.New(md5.New, []byte(a.password))
		h.Write(fromServer)
		digest := hex.EncodeToString(h.Sum(nil))
		response := fmt.Sprintf("%s %s", a.username, digest)
		return []byte(response), nil
	}
	return nil, nil
}

// LoginAuth implements LOGIN authentication
type loginAuth struct {
	username, password string
}

func LoginAuth(username, password string) smtp.Auth {
	return &loginAuth{username, password}
}

func (a *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", []byte{}, nil
}

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
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

// sendTestEmail sends a test email through an SMTP server
func (s *Server) sendTestEmail(c *fiber.Ctx) error {
	id := c.Params("id")

	var req struct {
		To      string `json:"to"`
		Subject string `json:"subject"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.To == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Email destination is required"})
	}

	if req.Subject == "" {
		req.Subject = "Email de Teste - SMTP Enviador"
	}

	// Get SMTP details
	var host, username, password, tlsMode, name string
	var port int

	err := s.db.QueryRow(`
		SELECT name, host, port, username, password, tls_mode FROM smtp_servers WHERE id = $1
	`, id).Scan(&name, &host, &port, &username, &password, &tlsMode)

	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "SMTP server not found"})
	}

	// Build email message
	from := username
	subject := req.Subject
	body := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: Arial, sans-serif; background: #f5f5f5; padding: 20px; }
        .container { max-width: 600px; margin: 0 auto; background: white; border-radius: 10px; padding: 40px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
        .header { text-align: center; margin-bottom: 30px; }
        .header h1 { color: #3b82f6; margin: 0; }
        .success { background: #10b981; color: white; padding: 15px 25px; border-radius: 8px; text-align: center; font-size: 18px; margin: 20px 0; }
        .details { background: #f8fafc; padding: 20px; border-radius: 8px; margin-top: 20px; }
        .details p { margin: 8px 0; color: #64748b; }
        .details strong { color: #334155; }
        .footer { text-align: center; margin-top: 30px; color: #94a3b8; font-size: 12px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>SMTP Enviador</h1>
        </div>
        <div class="success">
            ✓ Email de teste enviado com sucesso!
        </div>
        <p style="text-align: center; color: #64748b;">
            Este email confirma que seu servidor SMTP esta configurado corretamente.
        </p>
        <div class="details">
            <p><strong>Servidor SMTP:</strong> %s</p>
            <p><strong>Host:</strong> %s:%d</p>
            <p><strong>Usuario:</strong> %s</p>
            <p><strong>TLS:</strong> %s</p>
            <p><strong>Data/Hora:</strong> %s</p>
        </div>
        <div class="footer">
            <p>SMTP Enviador - Sistema de Email Marketing</p>
        </div>
    </div>
</body>
</html>`, name, host, port, username, tlsMode, time.Now().Format("02/01/2006 15:04:05"))

	// Build message with headers
	msg := fmt.Sprintf("From: %s\r\n", from)
	msg += fmt.Sprintf("To: %s\r\n", req.To)
	msg += fmt.Sprintf("Subject: %s\r\n", subject)
	msg += "MIME-Version: 1.0\r\n"
	msg += "Content-Type: text/html; charset=UTF-8\r\n"
	msg += "\r\n"
	msg += body

	// Connect and send
	addr := fmt.Sprintf("%s:%d", host, port)
	var client *smtp.Client

	// Handle different TLS modes
	switch tlsMode {
	case "tls":
		tlsConfig := &tls.Config{
			ServerName:         host,
			InsecureSkipVerify: true,
		}
		conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 15 * time.Second}, "tcp", addr, tlsConfig)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{
				"error":   "Falha na conexao TLS",
				"details": err.Error(),
			})
		}
		defer conn.Close()

		client, err = smtp.NewClient(conn, host)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{
				"error":   "Falha ao criar cliente SMTP",
				"details": err.Error(),
			})
		}

	case "starttls":
		conn, err := net.DialTimeout("tcp", addr, 15*time.Second)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{
				"error":   "Falha na conexao",
				"details": err.Error(),
			})
		}
		defer conn.Close()

		client, err = smtp.NewClient(conn, host)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{
				"error":   "Falha ao criar cliente SMTP",
				"details": err.Error(),
			})
		}

		if err := client.Hello("localhost"); err != nil {
			return c.Status(400).JSON(fiber.Map{
				"error":   "SMTP HELO falhou",
				"details": err.Error(),
			})
		}

		tlsConfig := &tls.Config{
			ServerName:         host,
			InsecureSkipVerify: true,
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			return c.Status(400).JSON(fiber.Map{
				"error":   "STARTTLS falhou",
				"details": err.Error(),
			})
		}

	default:
		conn, err := net.DialTimeout("tcp", addr, 15*time.Second)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{
				"error":   "Falha na conexao",
				"details": err.Error(),
			})
		}
		defer conn.Close()

		client, err = smtp.NewClient(conn, host)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{
				"error":   "Falha ao criar cliente SMTP",
				"details": err.Error(),
			})
		}

		if err := client.Hello("localhost"); err != nil {
			return c.Status(400).JSON(fiber.Map{
				"error":   "SMTP HELO falhou",
				"details": err.Error(),
			})
		}
	}
	defer client.Close()

	// Authenticate - try multiple methods
	authErr := client.Auth(CRAMMD5Auth(username, password))
	if authErr != nil {
		authErr = client.Auth(LoginAuth(username, password))
	}
	if authErr != nil {
		auth := smtp.PlainAuth("", username, password, host)
		authErr = client.Auth(auth)
	}
	if authErr != nil {
		return c.Status(400).JSON(fiber.Map{
			"error":   "Falha na autenticacao",
			"details": authErr.Error(),
		})
	}

	// Set sender
	if err := client.Mail(from); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error":   "Falha ao definir remetente",
			"details": err.Error(),
		})
	}

	// Set recipient
	if err := client.Rcpt(req.To); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error":   "Falha ao definir destinatario",
			"details": err.Error(),
		})
	}

	// Send message body
	w, err := client.Data()
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error":   "Falha ao iniciar envio",
			"details": err.Error(),
		})
	}

	_, err = w.Write([]byte(msg))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error":   "Falha ao escrever mensagem",
			"details": err.Error(),
		})
	}

	err = w.Close()
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error":   "Falha ao finalizar mensagem",
			"details": err.Error(),
		})
	}

	client.Quit()

	// Update SMTP status
	s.db.Exec(`UPDATE smtp_servers SET status = 'online', last_check = NOW() WHERE id = $1`, id)

	return c.JSON(fiber.Map{
		"message": "Email de teste enviado com sucesso!",
		"to":      req.To,
	})
}

// refreshSMTPs reloads SMTP servers into the engine
func (s *Server) refreshSMTPs(c *fiber.Ctx) error {
	if err := s.engine.RefreshSMTPs(); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to refresh SMTPs"})
	}
	return c.JSON(fiber.Map{"message": "SMTPs refreshed"})
}

// ============ SMTP SENDERS ============

type SenderRequest struct {
	Email   string `json:"email"`
	Name    string `json:"name"`
	ReplyTo string `json:"reply_to"`
	Active  bool   `json:"active"`
}

// listSMTPSenders returns all senders for an SMTP
func (s *Server) listSMTPSenders(c *fiber.Ctx) error {
	smtpID := c.Params("id")

	rows, err := s.db.Query(`
		SELECT id, email, name, reply_to, active, total_sent, created_at
		FROM smtp_senders
		WHERE smtp_id = $1
		ORDER BY created_at DESC
	`, smtpID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch senders"})
	}
	defer rows.Close()

	var senders []fiber.Map
	for rows.Next() {
		var id, email string
		var name, replyTo *string
		var active bool
		var totalSent int64
		var createdAt time.Time

		err := rows.Scan(&id, &email, &name, &replyTo, &active, &totalSent, &createdAt)
		if err != nil {
			continue
		}

		senders = append(senders, fiber.Map{
			"id":         id,
			"email":      email,
			"name":       name,
			"reply_to":   replyTo,
			"active":     active,
			"total_sent": totalSent,
			"created_at": createdAt,
		})
	}

	return c.JSON(fiber.Map{
		"data":  senders,
		"total": len(senders),
	})
}

// addSMTPSender adds a sender to an SMTP
func (s *Server) addSMTPSender(c *fiber.Ctx) error {
	smtpID := c.Params("id")

	var req SenderRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.Email == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Email is required"})
	}

	id := uuid.New().String()

	_, err := s.db.Exec(`
		INSERT INTO smtp_senders (id, smtp_id, email, name, reply_to, active)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, id, smtpID, req.Email, req.Name, req.ReplyTo, true)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to add sender (maybe duplicate?)"})
	}

	return c.Status(201).JSON(fiber.Map{
		"message": "Sender added",
		"id":      id,
	})
}

// addSMTPSendersBulk adds multiple senders to an SMTP
func (s *Server) addSMTPSendersBulk(c *fiber.Ctx) error {
	smtpID := c.Params("id")

	var req struct {
		Senders      []SenderRequest `json:"senders"`
		ClearExisting bool           `json:"clear_existing"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// Clear existing senders if requested
	if req.ClearExisting {
		s.db.Exec(`DELETE FROM smtp_senders WHERE smtp_id = $1`, smtpID)
	}

	added := 0
	for _, sender := range req.Senders {
		if sender.Email == "" {
			continue
		}
		id := uuid.New().String()
		_, err := s.db.Exec(`
			INSERT INTO smtp_senders (id, smtp_id, email, name, reply_to, active)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (smtp_id, email) DO UPDATE SET name = $4, reply_to = $5
		`, id, smtpID, sender.Email, sender.Name, sender.ReplyTo, true)
		if err == nil {
			added++
		}
	}

	// Refresh engine SMTP pool to load new senders
	s.engine.RefreshSMTPs()

	return c.Status(201).JSON(fiber.Map{
		"message": "Senders saved",
		"added":   added,
	})
}

// deleteSMTPSender removes a sender from an SMTP
func (s *Server) deleteSMTPSender(c *fiber.Ctx) error {
	senderID := c.Params("senderId")

	result, err := s.db.Exec(`DELETE FROM smtp_senders WHERE id = $1`, senderID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete sender"})
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "Sender not found"})
	}

	return c.JSON(fiber.Map{"message": "Sender deleted"})
}

// toggleSMTPSender toggles sender active status
func (s *Server) toggleSMTPSender(c *fiber.Ctx) error {
	senderID := c.Params("senderId")

	result, err := s.db.Exec(`
		UPDATE smtp_senders SET active = NOT active WHERE id = $1
	`, senderID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to toggle sender"})
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "Sender not found"})
	}

	return c.JSON(fiber.Map{"message": "Sender toggled"})
}
