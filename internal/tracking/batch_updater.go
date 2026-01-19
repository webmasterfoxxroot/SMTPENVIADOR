package tracking

import (
	"database/sql"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
)

// BatchUpdater handles batched database updates for tracking events
// This significantly reduces database load by accumulating updates
// and executing them in batches instead of one-by-one
type BatchUpdater struct {
	db            *sql.DB
	flushInterval time.Duration

	// Tracking events batch
	eventsMu sync.Mutex
	events   []TrackingEvent

	// Campaign email updates batch (opened_at, clicked_at)
	emailUpdatesMu sync.Mutex
	emailOpens     map[string]time.Time // email_id -> opened_at
	emailClicks    map[string]time.Time // email_id -> clicked_at

	// Campaign counter updates batch
	countersMu    sync.Mutex
	openCounters  map[string]int // campaign_id -> increment
	clickCounters map[string]int // campaign_id -> increment

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// TrackingEvent represents a tracking event to be inserted
type TrackingEvent struct {
	ID             string
	CampaignID     string
	EmailID        string
	EventType      string
	LinkURL        string
	IPAddress      string
	UserAgent      string
	Country        string
	CountryCode    string
	Region         string
	City           string
	Lat            float64
	Lon            float64
	Timezone       string
	ISP            string
	Browser        string
	BrowserVersion string
	OS             string
	OSVersion      string
	Device         string
	DeviceType     string
	EmailClient    string
	IsBot          bool
	IsSuspicious   bool
	BotType        string
	BotName        string
	BotScore       int
}

// NewBatchUpdater creates a new batch updater
func NewBatchUpdater(db *sql.DB, flushInterval time.Duration) *BatchUpdater {
	if flushInterval == 0 {
		flushInterval = 2 * time.Second // Default: flush every 2 seconds
	}

	b := &BatchUpdater{
		db:            db,
		flushInterval: flushInterval,
		events:        make([]TrackingEvent, 0, 1000),
		emailOpens:    make(map[string]time.Time),
		emailClicks:   make(map[string]time.Time),
		openCounters:  make(map[string]int),
		clickCounters: make(map[string]int),
		stopCh:        make(chan struct{}),
	}

	return b
}

// Start begins the background flush goroutine
func (b *BatchUpdater) Start() {
	b.wg.Add(1)
	go b.flushLoop()
	log.Printf("[BatchUpdater] Started with flush interval: %v", b.flushInterval)
}

// Stop gracefully stops the batch updater
func (b *BatchUpdater) Stop() {
	close(b.stopCh)
	b.wg.Wait()
	// Final flush
	b.Flush()
	log.Println("[BatchUpdater] Stopped")
}

// flushLoop runs the periodic flush
func (b *BatchUpdater) flushLoop() {
	defer b.wg.Done()
	ticker := time.NewTicker(b.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			b.Flush()
		case <-b.stopCh:
			return
		}
	}
}

// AddEvent adds a tracking event to the batch
func (b *BatchUpdater) AddEvent(event TrackingEvent) {
	if event.ID == "" {
		event.ID = uuid.New().String()
	}

	b.eventsMu.Lock()
	b.events = append(b.events, event)
	b.eventsMu.Unlock()
}

// AddOpen records an email open (will be batched)
func (b *BatchUpdater) AddOpen(campaignID, emailID string) {
	now := time.Now()

	b.emailUpdatesMu.Lock()
	key := campaignID + ":" + emailID
	if _, exists := b.emailOpens[key]; !exists {
		b.emailOpens[key] = now
	}
	b.emailUpdatesMu.Unlock()

	b.countersMu.Lock()
	b.openCounters[campaignID]++
	b.countersMu.Unlock()
}

// AddClick records an email click (will be batched)
func (b *BatchUpdater) AddClick(campaignID, emailID string) {
	now := time.Now()

	b.emailUpdatesMu.Lock()
	key := campaignID + ":" + emailID
	if _, exists := b.emailClicks[key]; !exists {
		b.emailClicks[key] = now
	}
	b.emailUpdatesMu.Unlock()

	b.countersMu.Lock()
	b.clickCounters[campaignID]++
	b.countersMu.Unlock()
}

// Flush executes all batched updates
func (b *BatchUpdater) Flush() {
	b.flushEvents()
	b.flushEmailUpdates()
	b.flushCounters()
}

// flushEvents inserts all batched tracking events
func (b *BatchUpdater) flushEvents() {
	b.eventsMu.Lock()
	if len(b.events) == 0 {
		b.eventsMu.Unlock()
		return
	}
	events := b.events
	b.events = make([]TrackingEvent, 0, 1000)
	b.eventsMu.Unlock()

	log.Printf("[BatchUpdater] Flushing %d tracking events", len(events))

	// Use a transaction for better performance
	tx, err := b.db.Begin()
	if err != nil {
		log.Printf("[BatchUpdater] Error starting transaction: %v", err)
		return
	}

	stmt, err := tx.Prepare(`
		INSERT INTO tracking_events (
			id, campaign_id, email_id, event_type, link_url,
			ip_address, user_agent,
			country, country_code, region, city, lat, lon, timezone, isp,
			browser, browser_version, os, os_version, device, device_type, email_client,
			is_bot, is_suspicious, bot_type, bot_name, bot_score
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27)
	`)
	if err != nil {
		tx.Rollback()
		log.Printf("[BatchUpdater] Error preparing statement: %v", err)
		return
	}
	defer stmt.Close()

	for _, e := range events {
		_, err := stmt.Exec(
			e.ID, e.CampaignID, e.EmailID, e.EventType, e.LinkURL,
			e.IPAddress, e.UserAgent,
			e.Country, e.CountryCode, e.Region, e.City, e.Lat, e.Lon, e.Timezone, e.ISP,
			e.Browser, e.BrowserVersion, e.OS, e.OSVersion, e.Device, e.DeviceType, e.EmailClient,
			e.IsBot, e.IsSuspicious, e.BotType, e.BotName, e.BotScore,
		)
		if err != nil {
			log.Printf("[BatchUpdater] Error inserting event: %v", err)
		}
	}

	if err := tx.Commit(); err != nil {
		log.Printf("[BatchUpdater] Error committing events: %v", err)
	}
}

// flushEmailUpdates updates opened_at and clicked_at in campaign_emails
func (b *BatchUpdater) flushEmailUpdates() {
	// Flush opens
	b.emailUpdatesMu.Lock()
	opens := b.emailOpens
	clicks := b.emailClicks
	b.emailOpens = make(map[string]time.Time)
	b.emailClicks = make(map[string]time.Time)
	b.emailUpdatesMu.Unlock()

	if len(opens) > 0 {
		log.Printf("[BatchUpdater] Flushing %d email open updates", len(opens))
		for key, openedAt := range opens {
			parts := splitKey(key)
			if len(parts) != 2 {
				continue
			}
			campaignID, emailID := parts[0], parts[1]

			_, err := b.db.Exec(`
				UPDATE campaign_emails
				SET opened_at = $3,
				    status = CASE WHEN status = 'queued' THEN 'sent' ELSE status END,
				    sent_at = CASE WHEN sent_at IS NULL THEN $3 ELSE sent_at END
				WHERE campaign_id = $1 AND email_id = $2 AND opened_at IS NULL
			`, campaignID, emailID, openedAt)

			if err != nil {
				log.Printf("[BatchUpdater] Error updating email open: %v", err)
			}
		}
	}

	if len(clicks) > 0 {
		log.Printf("[BatchUpdater] Flushing %d email click updates", len(clicks))
		for key, clickedAt := range clicks {
			parts := splitKey(key)
			if len(parts) != 2 {
				continue
			}
			campaignID, emailID := parts[0], parts[1]

			_, err := b.db.Exec(`
				UPDATE campaign_emails
				SET clicked_at = $3,
				    status = CASE WHEN status = 'queued' THEN 'sent' ELSE status END,
				    sent_at = CASE WHEN sent_at IS NULL THEN $3 ELSE sent_at END
				WHERE campaign_id = $1 AND email_id = $2 AND clicked_at IS NULL
			`, campaignID, emailID, clickedAt)

			if err != nil {
				log.Printf("[BatchUpdater] Error updating email click: %v", err)
			}
		}
	}
}

// flushCounters updates campaign counters
func (b *BatchUpdater) flushCounters() {
	b.countersMu.Lock()
	opens := b.openCounters
	clicks := b.clickCounters
	b.openCounters = make(map[string]int)
	b.clickCounters = make(map[string]int)
	b.countersMu.Unlock()

	// Batch update open counts
	for campaignID, count := range opens {
		if count == 0 {
			continue
		}
		_, err := b.db.Exec(`
			UPDATE campaigns SET open_count = open_count + $2 WHERE id = $1
		`, campaignID, count)
		if err != nil {
			log.Printf("[BatchUpdater] Error updating open count: %v", err)
		}
	}

	// Batch update click counts
	for campaignID, count := range clicks {
		if count == 0 {
			continue
		}
		_, err := b.db.Exec(`
			UPDATE campaigns SET click_count = click_count + $2 WHERE id = $1
		`, campaignID, count)
		if err != nil {
			log.Printf("[BatchUpdater] Error updating click count: %v", err)
		}
	}
}

// splitKey splits a "campaignID:emailID" key
func splitKey(key string) []string {
	for i := 0; i < len(key); i++ {
		if key[i] == ':' {
			return []string{key[:i], key[i+1:]}
		}
	}
	return nil
}

// Stats returns current batch sizes
func (b *BatchUpdater) Stats() (events, opens, clicks, openCounters, clickCounters int) {
	b.eventsMu.Lock()
	events = len(b.events)
	b.eventsMu.Unlock()

	b.emailUpdatesMu.Lock()
	opens = len(b.emailOpens)
	clicks = len(b.emailClicks)
	b.emailUpdatesMu.Unlock()

	b.countersMu.Lock()
	openCounters = len(b.openCounters)
	clickCounters = len(b.clickCounters)
	b.countersMu.Unlock()

	return
}
