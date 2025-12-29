package engine

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/tls"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
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

// SMTPSender represents a sender for an SMTP server
type SMTPSender struct {
	ID      string
	Email   string
	Name    string
	ReplyTo string
}

// SMTPConnection represents a connection to an SMTP server
type SMTPConnection struct {
	ID             string
	Name           string
	Host           string
	Port           int
	Username       string
	Password       string
	TLSMode        string // none, starttls, tls
	MaxPerMinute   int
	MaxPerHour     int
	MaxConnections int
	Active         bool
	Status         string
	Senders        []SMTPSender // List of senders for this SMTP
	client         *smtp.Client
	mu             sync.Mutex
	lastUsed       time.Time
	sentCount      int64
	senderIndex    int // For rotating senders
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
		SELECT id, name, host, port, username, password, tls_mode,
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
			&s.TLSMode, &s.MaxPerMinute, &s.MaxPerHour, &s.MaxConnections,
			&s.Active, &s.Status,
		)
		if err != nil {
			log.Printf("⚠️ Failed to scan SMTP server: %v", err)
			continue
		}

		// Load senders for this SMTP
		s.Senders = p.loadSenders(s.ID)
		p.servers = append(p.servers, &s)
		log.Printf("📧 Loaded SMTP: %s (%s:%d) TLS: %s, Senders: %d", s.Name, s.Host, s.Port, s.TLSMode, len(s.Senders))
	}

	log.Printf("✅ Loaded %d SMTP servers", len(p.servers))
	return nil
}

// loadSenders loads senders for a specific SMTP
func (p *SMTPPool) loadSenders(smtpID string) []SMTPSender {
	rows, err := p.db.Query(`
		SELECT id, email, name, reply_to
		FROM smtp_senders
		WHERE smtp_id = $1 AND active = true
	`, smtpID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var senders []SMTPSender
	for rows.Next() {
		var s SMTPSender
		var name, replyTo *string
		err := rows.Scan(&s.ID, &s.Email, &name, &replyTo)
		if err != nil {
			continue
		}
		if name != nil {
			s.Name = *name
		}
		if replyTo != nil {
			s.ReplyTo = *replyTo
		}
		senders = append(senders, s)
	}
	return senders
}

// GetNextSMTP returns the next available SMTP using round-robin (only SMTPs with senders)
func (p *SMTPPool) GetNextSMTP() *SMTPConnection {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if len(p.servers) == 0 {
		return nil
	}

	// Simple round-robin - only return SMTPs that have senders
	attempts := len(p.servers)
	for i := 0; i < attempts; i++ {
		idx := int(p.current.Add(1)) % len(p.servers)
		server := p.servers[idx]

		if server.Active && server.Status == "online" && len(server.Senders) > 0 {
			return server
		}
	}

	// If no online server with senders, return first active with senders
	for _, server := range p.servers {
		if server.Active && len(server.Senders) > 0 {
			return server
		}
	}

	return nil
}

// GetNextSender returns the next sender for this SMTP (round-robin)
func (s *SMTPConnection) GetNextSender() *SMTPSender {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.Senders) == 0 {
		return nil
	}

	sender := &s.Senders[s.senderIndex]
	s.senderIndex = (s.senderIndex + 1) % len(s.Senders)
	return sender
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
	headers["Subject"] = encodeRFC2047(params.Subject)
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

	// Handle different TLS modes
	switch s.TLSMode {
	case "tls":
		return s.sendWithImplicitTLS(addr, params.From, params.To, []byte(message))
	case "starttls":
		return s.sendWithSTARTTLS(addr, params.From, params.To, []byte(message))
	default: // "none" or empty
		return s.sendPlain(addr, params.From, params.To, []byte(message))
	}
}

// sendWithImplicitTLS sends email using implicit TLS (port 465)
func (s *SMTPConnection) sendWithImplicitTLS(addr, from, to string, msg []byte) error {
	tlsConfig := &tls.Config{
		ServerName:         s.Host,
		InsecureSkipVerify: true, // For self-signed certs
	}

	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 10 * time.Second}, "tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("failed to connect with TLS: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}
	defer client.Close()

	// Try authenticate
	if err := s.authenticate(client); err != nil {
		return err
	}

	return s.sendMessage(client, from, to, msg)
}

// sendWithSTARTTLS sends email using STARTTLS
func (s *SMTPConnection) sendWithSTARTTLS(addr, from, to string, msg []byte) error {
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

	// Say hello
	if err := client.Hello("localhost"); err != nil {
		return fmt.Errorf("failed to say hello: %w", err)
	}

	// STARTTLS
	tlsConfig := &tls.Config{
		ServerName:         s.Host,
		InsecureSkipVerify: true, // For self-signed certs
	}

	if err := client.StartTLS(tlsConfig); err != nil {
		return fmt.Errorf("failed to start TLS: %w", err)
	}

	// Try authenticate
	if err := s.authenticate(client); err != nil {
		return err
	}

	return s.sendMessage(client, from, to, msg)
}

// sendPlain sends email without TLS
func (s *SMTPConnection) sendPlain(addr, from, to string, msg []byte) error {
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

	// Say hello
	if err := client.Hello("localhost"); err != nil {
		return fmt.Errorf("failed to say hello: %w", err)
	}

	// Try authenticate
	if err := s.authenticate(client); err != nil {
		return err
	}

	return s.sendMessage(client, from, to, msg)
}

// authenticate tries CRAM-MD5 first, then LOGIN, then PLAIN
func (s *SMTPConnection) authenticate(client *smtp.Client) error {
	// Try CRAM-MD5 first (most secure, doesn't send password)
	if err := client.Auth(CRAMMD5Auth(s.Username, s.Password)); err == nil {
		return nil
	}

	// Try LOGIN auth (doesn't have unencrypted connection check)
	if err := client.Auth(LoginAuth(s.Username, s.Password)); err == nil {
		return nil
	}

	// Try PLAIN auth as last resort
	auth := smtp.PlainAuth("", s.Username, s.Password, s.Host)
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("failed to authenticate (tried CRAM-MD5, LOGIN, PLAIN): %w", err)
	}

	return nil
}

// sendMessage sends the email after authentication
func (s *SMTPConnection) sendMessage(client *smtp.Client, from, to string, msg []byte) error {
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("failed to set sender: %w", err)
	}

	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("failed to set recipient: %w", err)
	}

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
		// fromServer contains the challenge
		// Response is: username + space + HMAC-MD5(password, challenge)
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

// Close closes the SMTP connection
func (s *SMTPConnection) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.client != nil {
		s.client.Close()
		s.client = nil
	}
}

// formatAddress formats email address with name (RFC 2047 encoded)
func formatAddress(name, email string) string {
	if name == "" {
		return email
	}
	// Encode name with RFC 2047 Base64 encoding to handle UTF-8 characters
	encodedName := encodeRFC2047(name)
	return fmt.Sprintf("%s <%s>", encodedName, email)
}

// encodeRFC2047 encodes a string using RFC 2047 Base64 for email headers
func encodeRFC2047(s string) string {
	// Check if encoding is needed (non-ASCII characters)
	needsEncoding := false
	for _, r := range s {
		if r > 127 {
			needsEncoding = true
			break
		}
	}
	if !needsEncoding {
		return s
	}
	// Use Base64 encoding for UTF-8
	return "=?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte(s)) + "?="
}
