package models

import (
	"time"
)

// User represents a system user
type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Name         string    `json:"name"`
	Role         string    `json:"role"`
	Active       bool      `json:"active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// SMTPServer represents a PowerMTA SMTP server
type SMTPServer struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Host           string    `json:"host"`
	Port           int       `json:"port"`
	Username       string    `json:"username"`
	Password       string    `json:"password,omitempty"`
	TLS            bool      `json:"tls"`
	MaxPerMinute   int       `json:"max_per_minute"`
	MaxPerHour     int       `json:"max_per_hour"`
	MaxConnections int       `json:"max_connections"`
	Active         bool      `json:"active"`
	Status         string    `json:"status"` // online, offline, error
	LastCheck      time.Time `json:"last_check"`
	TotalSent      int64     `json:"total_sent"`
	TotalFailed    int64     `json:"total_failed"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// EmailList represents a mailing list
type EmailList struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	TotalEmails   int       `json:"total_emails"`
	ValidEmails   int       `json:"valid_emails"`
	InvalidEmails int       `json:"invalid_emails"`
	Status        string    `json:"status"` // pending, processing, ready, error
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Email represents a subscriber in a list
type Email struct {
	ID           string    `json:"id"`
	ListID       string    `json:"list_id"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	Custom1      string    `json:"custom1,omitempty"`
	Custom2      string    `json:"custom2,omitempty"`
	Custom3      string    `json:"custom3,omitempty"`
	Custom4      string    `json:"custom4,omitempty"`
	Custom5      string    `json:"custom5,omitempty"`
	Custom6      string    `json:"custom6,omitempty"`
	Custom7      string    `json:"custom7,omitempty"`
	Custom8      string    `json:"custom8,omitempty"`
	Custom9      string    `json:"custom9,omitempty"`
	Custom10     string    `json:"custom10,omitempty"`
	Valid        bool      `json:"valid"`
	Bounced      bool      `json:"bounced"`
	Unsubscribed bool      `json:"unsubscribed"`
	CreatedAt    time.Time `json:"created_at"`
}

// Template represents an email template
type Template struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Subject     string    `json:"subject"`
	HTMLContent string    `json:"html_content"`
	TextContent string    `json:"text_content"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Campaign represents an email campaign
type Campaign struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	Subject          string     `json:"subject"`
	FromName         string     `json:"from_name"`
	FromEmail        string     `json:"from_email"`
	ReplyTo          string     `json:"reply_to"`
	HTMLContent      string     `json:"html_content"`
	TextContent      string     `json:"text_content"`
	ListID           string     `json:"list_id"`
	Status           string     `json:"status"` // draft, scheduled, running, paused, completed, cancelled
	ScheduledAt      *time.Time `json:"scheduled_at,omitempty"`
	StartedAt        *time.Time `json:"started_at,omitempty"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
	TotalEmails      int        `json:"total_emails"`
	SentCount        int        `json:"sent_count"`
	FailedCount      int        `json:"failed_count"`
	OpenCount        int        `json:"open_count"`
	ClickCount       int        `json:"click_count"`
	BounceCount      int        `json:"bounce_count"`
	UnsubscribeCount int        `json:"unsubscribe_count"`
	SendRate         int        `json:"send_rate"` // emails per minute
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// CampaignEmail represents an email in a campaign queue
type CampaignEmail struct {
	ID           string     `json:"id"`
	CampaignID   string     `json:"campaign_id"`
	EmailID      string     `json:"email_id"`
	SMTPID       string     `json:"smtp_id"`
	Status       string     `json:"status"` // pending, queued, sending, sent, failed, bounced
	ErrorMessage string     `json:"error_message,omitempty"`
	SentAt       *time.Time `json:"sent_at,omitempty"`
	OpenedAt     *time.Time `json:"opened_at,omitempty"`
	ClickedAt    *time.Time `json:"clicked_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

// TrackingEvent represents a tracking event
type TrackingEvent struct {
	ID         string    `json:"id"`
	CampaignID string    `json:"campaign_id"`
	EmailID    string    `json:"email_id"`
	EventType  string    `json:"event_type"` // open, click, bounce, unsubscribe
	LinkURL    string    `json:"link_url,omitempty"`
	IPAddress  string    `json:"ip_address"`
	UserAgent  string    `json:"user_agent"`
	Country    string    `json:"country"`
	City       string    `json:"city"`
	CreatedAt  time.Time `json:"created_at"`
}

// Blacklist represents a blacklisted email
type Blacklist struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Reason    string    `json:"reason"` // bounce, unsubscribe, complaint, manual
	CreatedAt time.Time `json:"created_at"`
}

// Stats represents dashboard statistics
type Stats struct {
	TotalSent      int64   `json:"total_sent"`
	TotalFailed    int64   `json:"total_failed"`
	TotalOpened    int64   `json:"total_opened"`
	TotalClicked   int64   `json:"total_clicked"`
	TotalBounced   int64   `json:"total_bounced"`
	QueueSize      int64   `json:"queue_size"`
	SendingRate    float64 `json:"sending_rate"` // emails per second
	ActiveSMTPs    int     `json:"active_smtps"`
	ActiveCampaigns int    `json:"active_campaigns"`
}
