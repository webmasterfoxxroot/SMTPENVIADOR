package clickhouse

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"smtpenviador/internal/config"
)

// Client wraps the ClickHouse connection
type Client struct {
	conn driver.Conn
	cfg  *config.Config
}

// Connect creates a new ClickHouse connection
func Connect(cfg *config.Config) (*Client, error) {
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{fmt.Sprintf("%s:%s", cfg.ClickHouseHost, cfg.ClickHousePort)},
		Auth: clickhouse.Auth{
			Database: cfg.ClickHouseDB,
			Username: cfg.ClickHouseUser,
			Password: cfg.ClickHousePassword,
		},
		Settings: clickhouse.Settings{
			"max_execution_time": 60,
		},
		DialTimeout:     10 * time.Second,
		MaxOpenConns:    50,
		MaxIdleConns:    25,
		ConnMaxLifetime: time.Hour,
		Compression: &clickhouse.Compression{
			Method: clickhouse.CompressionLZ4,
		},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to connect to ClickHouse: %w", err)
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := conn.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping ClickHouse: %w", err)
	}

	log.Println("✅ ClickHouse connected")

	return &Client{conn: conn, cfg: cfg}, nil
}

// Close closes the ClickHouse connection
func (c *Client) Close() error {
	return c.conn.Close()
}

// Conn returns the underlying connection
func (c *Client) Conn() driver.Conn {
	return c.conn
}

// ===========================================
// BLACKLIST OPERATIONS
// ===========================================

// InsertBlacklistBatch inserts multiple emails into blacklist
func (c *Client) InsertBlacklistBatch(ctx context.Context, emails []string, reason string) (inserted int64, duplicates int64, err error) {
	if len(emails) == 0 {
		return 0, 0, nil
	}

	batch, err := c.conn.PrepareBatch(ctx, `
		INSERT INTO blacklist (email, reason, created_at)
	`)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to prepare batch: %w", err)
	}

	for _, email := range emails {
		if err := batch.Append(email, reason, time.Now()); err != nil {
			log.Printf("Failed to append email %s: %v", email, err)
			continue
		}
		inserted++
	}

	if err := batch.Send(); err != nil {
		return 0, 0, fmt.Errorf("failed to send batch: %w", err)
	}

	return inserted, 0, nil
}

// IsBlacklisted checks if an email is in the blacklist
func (c *Client) IsBlacklisted(ctx context.Context, email string) (bool, error) {
	var count uint64
	err := c.conn.QueryRow(ctx, `
		SELECT count() FROM blacklist WHERE lower(email) = lower($1)
	`, email).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// IsBlacklistedBatch checks multiple emails against blacklist
func (c *Client) IsBlacklistedBatch(ctx context.Context, emails []string) (map[string]bool, error) {
	result := make(map[string]bool)
	if len(emails) == 0 {
		return result, nil
	}

	rows, err := c.conn.Query(ctx, `
		SELECT lower(email) FROM blacklist WHERE lower(email) IN ($1)
	`, emails)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			continue
		}
		result[email] = true
	}

	return result, nil
}

// GetBlacklistCount returns total count of blacklisted emails
func (c *Client) GetBlacklistCount(ctx context.Context) (uint64, error) {
	var count uint64
	err := c.conn.QueryRow(ctx, `SELECT count() FROM blacklist`).Scan(&count)
	return count, err
}

// ClearBlacklist removes all entries from blacklist
func (c *Client) ClearBlacklist(ctx context.Context) error {
	return c.conn.Exec(ctx, `TRUNCATE TABLE blacklist`)
}

// SearchBlacklist searches for emails in blacklist
func (c *Client) SearchBlacklist(ctx context.Context, search string, limit, offset int) ([]BlacklistEntry, uint64, error) {
	var total uint64
	var entries []BlacklistEntry

	// Get total count
	if search != "" {
		err := c.conn.QueryRow(ctx, `
			SELECT count() FROM blacklist WHERE email LIKE $1
		`, "%"+search+"%").Scan(&total)
		if err != nil {
			return nil, 0, err
		}
	} else {
		var err error
		total, err = c.GetBlacklistCount(ctx)
		if err != nil {
			return nil, 0, err
		}
	}

	// Get entries
	var rows driver.Rows
	var err error
	if search != "" {
		rows, err = c.conn.Query(ctx, `
			SELECT id, email, reason, created_at
			FROM blacklist
			WHERE email LIKE $1
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3
		`, "%"+search+"%", limit, offset)
	} else {
		rows, err = c.conn.Query(ctx, `
			SELECT id, email, reason, created_at
			FROM blacklist
			ORDER BY created_at DESC
			LIMIT $1 OFFSET $2
		`, limit, offset)
	}
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var entry BlacklistEntry
		if err := rows.Scan(&entry.ID, &entry.Email, &entry.Reason, &entry.CreatedAt); err != nil {
			continue
		}
		entries = append(entries, entry)
	}

	return entries, total, nil
}

// DeleteFromBlacklist removes an email from blacklist by ID
func (c *Client) DeleteFromBlacklist(ctx context.Context, id string) error {
	return c.conn.Exec(ctx, `ALTER TABLE blacklist DELETE WHERE id = $1`, id)
}

// ===========================================
// EMAIL LIST OPERATIONS
// ===========================================

// InsertEmailsBatch inserts multiple emails into a list
func (c *Client) InsertEmailsBatch(ctx context.Context, listID string, emails []EmailEntry) (inserted int64, err error) {
	if len(emails) == 0 {
		return 0, nil
	}

	batch, err := c.conn.PrepareBatch(ctx, `
		INSERT INTO emails (list_id, email, name, custom1, custom2, custom3, custom4, custom5, valid, created_at)
	`)
	if err != nil {
		return 0, fmt.Errorf("failed to prepare batch: %w", err)
	}

	for _, e := range emails {
		if err := batch.Append(
			listID,
			e.Email,
			e.Name,
			e.Custom1,
			e.Custom2,
			e.Custom3,
			e.Custom4,
			e.Custom5,
			e.Valid,
			time.Now(),
		); err != nil {
			log.Printf("Failed to append email %s: %v", e.Email, err)
			continue
		}
		inserted++
	}

	if err := batch.Send(); err != nil {
		return 0, fmt.Errorf("failed to send batch: %w", err)
	}

	return inserted, nil
}

// GetEmailCountByList returns count of emails in a list
func (c *Client) GetEmailCountByList(ctx context.Context, listID string) (uint64, error) {
	var count uint64
	err := c.conn.QueryRow(ctx, `
		SELECT count() FROM emails WHERE list_id = $1
	`, listID).Scan(&count)
	return count, err
}

// GetEmailsByList returns emails from a list with pagination
func (c *Client) GetEmailsByList(ctx context.Context, listID string, limit, offset int) ([]EmailEntry, uint64, error) {
	total, err := c.GetEmailCountByList(ctx, listID)
	if err != nil {
		return nil, 0, err
	}

	rows, err := c.conn.Query(ctx, `
		SELECT id, email, name, custom1, custom2, custom3, valid, bounced, unsubscribed, created_at
		FROM emails
		WHERE list_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, listID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var emails []EmailEntry
	for rows.Next() {
		var e EmailEntry
		if err := rows.Scan(&e.ID, &e.Email, &e.Name, &e.Custom1, &e.Custom2, &e.Custom3, &e.Valid, &e.Bounced, &e.Unsubscribed, &e.CreatedAt); err != nil {
			continue
		}
		emails = append(emails, e)
	}

	return emails, total, nil
}

// DeleteEmailsByList removes all emails from a list
func (c *Client) DeleteEmailsByList(ctx context.Context, listID string) error {
	return c.conn.Exec(ctx, `ALTER TABLE emails DELETE WHERE list_id = $1`, listID)
}

// MarkEmailsAsInvalid marks emails as invalid based on blacklist
func (c *Client) MarkEmailsAsInvalid(ctx context.Context, listID string) error {
	return c.conn.Exec(ctx, `
		ALTER TABLE emails UPDATE valid = 0
		WHERE list_id = $1 AND lower(email) IN (SELECT lower(email) FROM blacklist)
	`, listID)
}

// ===========================================
// TYPES
// ===========================================

type BlacklistEntry struct {
	ID        string
	Email     string
	Reason    string
	CreatedAt time.Time
}

type EmailEntry struct {
	ID           string
	ListID       string
	Email        string
	Name         string
	Custom1      string
	Custom2      string
	Custom3      string
	Custom4      string
	Custom5      string
	Valid        bool
	Bounced      bool
	Unsubscribed bool
	CreatedAt    time.Time
}
