package engine

import (
	"crypto/tls"
	"database/sql"
	"fmt"
	"log"
	"net"
	"net/smtp"
	"sync"
	"sync/atomic"
	"time"
)

// SMTPPool manages SMTP server connections
type SMTPPool struct {
	db       *sql.DB
	servers  []*SMTPConnection
	mu       sync.RWMutex
	current  atomic.Int32
	stopChan chan struct{}
}

// SMTPConnection represents a connection to an SMTP server
type SMTPConnection struct {
	ID             string
	Name           string
	Host           string
	Port           int
	Username       string
	Password       string
	TLS            bool
	MaxPerMinute   int
	MaxPerHour     int
	MaxConnections int
	Active         bool
	Status         string
	client         *smtp.Client
	mu             sync.Mutex
	lastUsed       time.Time
	sentCount      int64
}

// SendParams holds email parameters
type SendParams struct {
	From        string
	FromName    string
	To          string
	ToName      string
	ReplyTo     string
	Subject     string
	HTMLContent string
	TextContent string
}

// NewSMTPPool creates a new SMTP pool
func NewSMTPPool(db *sql.DB) *SMTPPool {
	return &SMTPPool{
		db:       db,
		servers:  make([]*SMTPConnection, 0),
		stopChan: make(chan struct{}),
	}
}

// LoadServers loads SMTP servers from database
func (p *SMTPPool) LoadServers() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	rows, err := p.db.Query(`
		SELECT id, name, host, port, username, password, tls,
		       max_per_minute, max_per_hour, max_connections, active, status
		FROM smtp_servers WHERE active = true
	`)
	if err != nil {
		return fmt.Errorf("failed to load SMTP servers: %w", err)
	}
	defer rows.Close()

	// Close existing connections
	for _, s := range p.servers {
		s.Close()
	}
	p.servers = make([]*SMTPConnection, 0)

	for rows.Next() {
		var s SMTPConnection
		err := rows.Scan(
			&s.ID, &s.Name, &s.Host, &s.Port, &s.Username, &s.Password,
			&s.TLS, &s.MaxPerMinute, &s.MaxPerHour, &s.MaxConnections,
			&s.Active, &s.Status,
		)
		if err != nil {
			log.Printf("⚠️ Failed to scan SMTP server: %v", err)
			continue
		}
		p.servers = append(p.servers, &s)
		log.Printf("📧 Loaded SMTP: %s (%s:%d)", s.Name, s.Host, s.Port)
	}

	log.Printf("✅ Loaded %d SMTP servers", len(p.servers))
	return nil
}

// GetNextSMTP returns the next available SMTP using round-robin
func (p *SMTPPool) GetNextSMTP() *SMTPConnection {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if len(p.servers) == 0 {
		return nil
	}

	// Simple round-robin
	attempts := len(p.servers)
	for i := 0; i < attempts; i++ {
		idx := int(p.current.Add(1)) % len(p.servers)
		server := p.servers[idx]

		if server.Active && server.Status == "online" {
			return server
		}
	}

	// If no online server, return first active
	for _, server := range p.servers {
		if server.Active {
			return server
		}
	}

	return nil
}

// GetActiveCount returns number of active SMTP servers
func (p *SMTPPool) GetActiveCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	count := 0
	for _, s := range p.servers {
		if s.Active && s.Status == "online" {
			count++
		}
	}
	return count
}

// StartHealthChecker starts the health check loop
func (p *SMTPPool) StartHealthChecker() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Initial check
	p.checkAllServers()

	for {
		select {
		case <-p.stopChan:
			return
		case <-ticker.C:
			p.checkAllServers()
		}
	}
}

// checkAllServers checks health of all SMTP servers
func (p *SMTPPool) checkAllServers() {
	p.mu.RLock()
	servers := make([]*SMTPConnection, len(p.servers))
	copy(servers, p.servers)
	p.mu.RUnlock()

	for _, server := range servers {
		go p.checkServer(server)
	}
}

// checkServer checks if an SMTP server is reachable
func (p *SMTPPool) checkServer(s *SMTPConnection) {
	addr := fmt.Sprintf("%s:%d", s.Host, s.Port)

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		p.updateServerStatus(s.ID, "offline")
		log.Printf("❌ SMTP %s is offline: %v", s.Name, err)
		return
	}
	conn.Close()

	p.updateServerStatus(s.ID, "online")
}

// updateServerStatus updates server status in DB
func (p *SMTPPool) updateServerStatus(id, status string) {
	p.db.Exec(`UPDATE smtp_servers SET status = $1, last_check = NOW() WHERE id = $2`, status, id)

	p.mu.Lock()
	for _, s := range p.servers {
		if s.ID == id {
			s.Status = status
			break
		}
	}
	p.mu.Unlock()
}

// CloseAll closes all SMTP connections
func (p *SMTPPool) CloseAll() {
	close(p.stopChan)

	p.mu.Lock()
	defer p.mu.Unlock()

	for _, s := range p.servers {
		s.Close()
	}
}

// Send sends an email through this SMTP connection
func (s *SMTPConnection) Send(params SendParams) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	addr := fmt.Sprintf("%s:%d", s.Host, s.Port)

	// Create message
	headers := make(map[string]string)
	headers["From"] = formatAddress(params.FromName, params.From)
	headers["To"] = formatAddress(params.ToName, params.To)
	headers["Subject"] = params.Subject
	headers["MIME-Version"] = "1.0"
	headers["Content-Type"] = `multipart/alternative; boundary="boundary-smtpenviador"`

	if params.ReplyTo != "" {
		headers["Reply-To"] = params.ReplyTo
	}

	// Build message
	message := ""
	for k, v := range headers {
		message += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	message += "\r\n"

	// Text part
	message += "--boundary-smtpenviador\r\n"
	message += "Content-Type: text/plain; charset=\"UTF-8\"\r\n\r\n"
	message += params.TextContent + "\r\n"

	// HTML part
	message += "--boundary-smtpenviador\r\n"
	message += "Content-Type: text/html; charset=\"UTF-8\"\r\n\r\n"
	message += params.HTMLContent + "\r\n"
	message += "--boundary-smtpenviador--"

	// Auth
	auth := smtp.PlainAuth("", s.Username, s.Password, s.Host)

	// Send with TLS
	if s.TLS {
		return s.sendWithTLS(addr, auth, params.From, params.To, []byte(message))
	}

	return smtp.SendMail(addr, auth, params.From, []string{params.To}, []byte(message))
}

// sendWithTLS sends email using STARTTLS
func (s *SMTPConnection) sendWithTLS(addr string, auth smtp.Auth, from, to string, msg []byte) error {
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	client, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to create client: %w", err)
	}
	defer client.Close()

	// STARTTLS
	tlsConfig := &tls.Config{
		ServerName:         s.Host,
		InsecureSkipVerify: true, // For self-signed certs
	}

	if err := client.StartTLS(tlsConfig); err != nil {
		return fmt.Errorf("failed to start TLS: %w", err)
	}

	// Auth
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("failed to authenticate: %w", err)
	}

	// Set sender and recipient
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("failed to set sender: %w", err)
	}

	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("failed to set recipient: %w", err)
	}

	// Send message
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("failed to get data writer: %w", err)
	}

	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("failed to close writer: %w", err)
	}

	return client.Quit()
}

// Close closes the SMTP connection
func (s *SMTPConnection) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.client != nil {
		s.client.Close()
		s.client = nil
	}
}

// formatAddress formats email address with name
func formatAddress(name, email string) string {
	if name == "" {
		return email
	}
	return fmt.Sprintf("%s <%s>", name, email)
}
