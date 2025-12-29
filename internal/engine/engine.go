package engine

import (
	"database/sql"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"smtpenviador/internal/config"
	"smtpenviador/internal/queue"
)

// Engine is the main email sending engine
type Engine struct {
	cfg          *config.Config
	db           *sql.DB
	queue        *queue.Manager
	smtpPool     *SMTPPool
	workers      []*Worker
	running      atomic.Bool
	wg           sync.WaitGroup
	stats        *EngineStats
	stopChan     chan struct{}
}

// EngineStats holds real-time statistics
type EngineStats struct {
	TotalSent     atomic.Int64
	TotalFailed   atomic.Int64
	SendingRate   atomic.Int64 // emails per second in last minute
	ActiveWorkers atomic.Int32
	StartTime     time.Time
	mu            sync.RWMutex
	sentPerSecond []int64
}

// New creates a new email engine
func New(cfg *config.Config, db *sql.DB, queueManager *queue.Manager) *Engine {
	return &Engine{
		cfg:      cfg,
		db:       db,
		queue:    queueManager,
		smtpPool: NewSMTPPool(db),
		stats:    &EngineStats{StartTime: time.Now()},
		stopChan: make(chan struct{}),
	}
}

// Start starts the email engine
func (e *Engine) Start() {
	if e.running.Load() {
		return
	}
	e.running.Store(true)

	log.Printf("⚡ Engine starting with %d workers", e.cfg.WorkersCount)

	// Load SMTP servers
	if err := e.smtpPool.LoadServers(); err != nil {
		log.Printf("⚠️ Warning: Failed to load SMTP servers: %v", err)
	}

	// Start SMTP health checker
	go e.smtpPool.StartHealthChecker()

	// Start workers
	e.workers = make([]*Worker, e.cfg.WorkersCount)
	for i := 0; i < e.cfg.WorkersCount; i++ {
		e.workers[i] = NewWorker(i, e.cfg, e.db, e.queue, e.smtpPool, e.stats)
		e.wg.Add(1)
		go func(w *Worker) {
			defer e.wg.Done()
			w.Start(e.stopChan)
		}(e.workers[i])
	}

	// Start stats calculator
	go e.calculateStats()

	log.Println("⚡ Engine running")
}

// Stop stops the email engine
func (e *Engine) Stop() {
	if !e.running.Load() {
		return
	}

	log.Println("🛑 Stopping engine...")
	e.running.Store(false)
	close(e.stopChan)

	// Wait for workers to finish
	e.wg.Wait()

	// Close SMTP connections
	e.smtpPool.CloseAll()

	log.Println("✅ Engine stopped")
}

// IsRunning returns whether the engine is running
func (e *Engine) IsRunning() bool {
	return e.running.Load()
}

// GetStats returns current engine statistics
func (e *Engine) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"total_sent":     e.stats.TotalSent.Load(),
		"total_failed":   e.stats.TotalFailed.Load(),
		"sending_rate":   e.stats.SendingRate.Load(),
		"active_workers": e.stats.ActiveWorkers.Load(),
		"uptime_seconds": time.Since(e.stats.StartTime).Seconds(),
		"active_smtps":   e.smtpPool.GetActiveCount(),
	}
}

// RefreshSMTPs reloads SMTP servers from database
func (e *Engine) RefreshSMTPs() error {
	return e.smtpPool.LoadServers()
}

// calculateStats calculates sending rate
func (e *Engine) calculateStats() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	var lastSent int64

	for {
		select {
		case <-e.stopChan:
			return
		case <-ticker.C:
			currentSent := e.stats.TotalSent.Load()
			rate := currentSent - lastSent
			e.stats.SendingRate.Store(rate)
			lastSent = currentSent
		}
	}
}
