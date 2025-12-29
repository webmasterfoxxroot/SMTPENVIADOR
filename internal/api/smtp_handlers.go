package api

import (
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
	TLS            bool   `json:"tls"`
	MaxPerMinute   int    `json:"max_per_minute"`
	MaxPerHour     int    `json:"max_per_hour"`
	MaxConnections int    `json:"max_connections"`
	Active         bool   `json:"active"`
}

// listSMTPs returns all SMTP servers
func (s *Server) listSMTPs(c *fiber.Ctx) error {
	rows, err := s.db.Query(`
		SELECT id, name, host, port, username, tls,
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
		var id, name, host, username, status string
		var port, maxPerMinute, maxPerHour, maxConnections int
		var tls, active bool
		var totalSent, totalFailed int64
		var lastCheck, createdAt, updatedAt *time.Time

		err := rows.Scan(&id, &name, &host, &port, &username, &tls,
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
			"tls":             tls,
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
		INSERT INTO smtp_servers (id, name, host, port, username, password, tls,
		                          max_per_minute, max_per_hour, max_connections, active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, id, req.Name, req.Host, req.Port, req.Username, req.Password, req.TLS,
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

	var name, host, username, status string
	var port, maxPerMinute, maxPerHour, maxConnections int
	var tls, active bool
	var totalSent, totalFailed int64
	var lastCheck, createdAt, updatedAt *time.Time

	err := s.db.QueryRow(`
		SELECT name, host, port, username, tls,
		       max_per_minute, max_per_hour, max_connections,
		       active, status, last_check, total_sent, total_failed,
		       created_at, updated_at
		FROM smtp_servers WHERE id = $1
	`, id).Scan(&name, &host, &port, &username, &tls,
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
		"tls":             tls,
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

	query := `
		UPDATE smtp_servers SET
			name = $1, host = $2, port = $3, username = $4,
			tls = $5, max_per_minute = $6, max_per_hour = $7,
			max_connections = $8, active = $9
		WHERE id = $10
	`
	args := []interface{}{req.Name, req.Host, req.Port, req.Username,
		req.TLS, req.MaxPerMinute, req.MaxPerHour, req.MaxConnections, req.Active, id}

	// If password provided, update it too
	if req.Password != "" {
		query = `
			UPDATE smtp_servers SET
				name = $1, host = $2, port = $3, username = $4, password = $5,
				tls = $6, max_per_minute = $7, max_per_hour = $8,
				max_connections = $9, active = $10
			WHERE id = $11
		`
		args = []interface{}{req.Name, req.Host, req.Port, req.Username, req.Password,
			req.TLS, req.MaxPerMinute, req.MaxPerHour, req.MaxConnections, req.Active, id}
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

	var host, username, password string
	var port int
	var tls bool

	err := s.db.QueryRow(`
		SELECT host, port, username, password, tls FROM smtp_servers WHERE id = $1
	`, id).Scan(&host, &port, &username, &password, &tls)

	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "SMTP server not found"})
	}

	// Test connection
	addr := fmt.Sprintf("%s:%d", host, port)
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		s.db.Exec(`UPDATE smtp_servers SET status = 'offline', last_check = NOW() WHERE id = $1`, id)
		return c.Status(400).JSON(fiber.Map{
			"error":   "Connection failed",
			"details": err.Error(),
		})
	}
	defer conn.Close()

	// Test SMTP AUTH
	auth := smtp.PlainAuth("", username, password, host)
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		s.db.Exec(`UPDATE smtp_servers SET status = 'error', last_check = NOW() WHERE id = $1`, id)
		return c.Status(400).JSON(fiber.Map{
			"error":   "SMTP client failed",
			"details": err.Error(),
		})
	}
	defer client.Close()

	if err := client.Auth(auth); err != nil {
		s.db.Exec(`UPDATE smtp_servers SET status = 'error', last_check = NOW() WHERE id = $1`, id)
		return c.Status(400).JSON(fiber.Map{
			"error":   "Authentication failed",
			"details": err.Error(),
		})
	}

	// Update status to online
	s.db.Exec(`UPDATE smtp_servers SET status = 'online', last_check = NOW() WHERE id = $1`, id)

	return c.JSON(fiber.Map{
		"message": "SMTP connection successful",
		"status":  "online",
	})
}

// refreshSMTPs reloads SMTP servers into the engine
func (s *Server) refreshSMTPs(c *fiber.Ctx) error {
	if err := s.engine.RefreshSMTPs(); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to refresh SMTPs"})
	}
	return c.JSON(fiber.Map{"message": "SMTPs refreshed"})
}
