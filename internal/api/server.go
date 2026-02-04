package api

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/websocket/v2"

	"smtpenviador/internal/clickhouse"
	"smtpenviador/internal/config"
	"smtpenviador/internal/engine"
	"smtpenviador/internal/queue"
	"smtpenviador/internal/tracking"
)

// Server represents the API server
type Server struct {
	app          *fiber.App
	cfg          *config.Config
	db           *sql.DB            // PostgreSQL for relational data
	ch           *clickhouse.Client // ClickHouse for bulk data (emails, blacklist)
	queue        *queue.Manager
	engine       *engine.Engine
	batchUpdater *tracking.BatchUpdater // Batch updater for tracking events
}

// NewServer creates a new API server
func NewServer(cfg *config.Config, db *sql.DB, ch *clickhouse.Client, q *queue.Manager, eng *engine.Engine) *Server {
	app := fiber.New(fiber.Config{
		AppName:      "SMTPENVIADOR API",
		ReadTimeout:  10 * time.Minute,  // 10 min for large file uploads
		WriteTimeout: 10 * time.Minute,  // 10 min for large file uploads
		BodyLimit:    500 * 1024 * 1024, // 500MB for list uploads
	})

	// Initialize batch updater for tracking events (flushes every 2 seconds)
	batchUpdater := tracking.NewBatchUpdater(db, 2*time.Second)
	batchUpdater.Start()

	server := &Server{
		app:          app,
		cfg:          cfg,
		db:           db,
		ch:           ch,
		queue:        q,
		engine:       eng,
		batchUpdater: batchUpdater,
	}

	server.setupMiddlewares()
	server.setupRoutes()

	// Start background scheduler for auto-starting campaigns
	go server.runAutoStartScheduler()

	// Start background job to auto-requeue running campaigns with empty queues
	go server.runAutoRequeueScheduler()

	// Resume any pending/orphaned import jobs from before restart
	go server.resumeOrphanedImportJobs()

	// Initialize warmup tables and start warmup engine
	server.initWarmupTables()
	go server.startWarmupEngine()

	return server
}

// runAutoStartScheduler runs a background scheduler that checks for campaigns to auto-start
func (s *Server) runAutoStartScheduler() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		s.checkAndAutoStartCampaigns()
		s.checkAndStartScheduledCampaigns()
	}
}

// checkAndAutoStartCampaigns checks for campaigns with passed auto_start_at and starts them
func (s *Server) checkAndAutoStartCampaigns() {
	// Use EXTRACT(EPOCH) to get Unix timestamp directly from PostgreSQL
	// This avoids any timezone conversion issues
	nowUnix := time.Now().Unix()
	rows, err := s.db.Query(`
		SELECT id, EXTRACT(EPOCH FROM auto_start_at)::bigint as auto_start_unix
		FROM campaigns
		WHERE status = 'draft'
		AND auto_start_at IS NOT NULL
	`)
	if err != nil {
		fmt.Printf("[AutoStart] Error querying campaigns: %v\n", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		var autoStartUnix int64
		if err := rows.Scan(&id, &autoStartUnix); err != nil {
			fmt.Printf("[AutoStart] Scan error: %v\n", err)
			continue
		}

		// Compare Unix timestamps directly
		if autoStartUnix <= nowUnix {
			fmt.Printf("[AutoStart] Starting campaign %s (target: %d, now: %d)\n", id, autoStartUnix, nowUnix)
			s.autoStartCampaignByID(id)
		}
	}
}

// runAutoRequeueScheduler periodically checks for running campaigns with empty queues and requeues them
func (s *Server) runAutoRequeueScheduler() {
	// Wait 10 seconds on startup to let system stabilize
	time.Sleep(10 * time.Second)

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		s.checkAndRequeueRunningCampaigns()
	}
}

// checkAndRequeueRunningCampaigns finds running campaigns with pending emails but empty queues
func (s *Server) checkAndRequeueRunningCampaigns() {
	// Find campaigns that are "running" but have pending emails (sent_count < total_emails)
	rows, err := s.db.Query(`
		SELECT id, name, sent_count, failed_count, total_emails
		FROM campaigns
		WHERE status = 'running'
		AND (sent_count + failed_count) < total_emails
	`)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id, name string
		var sentCount, failedCount, totalEmails int

		if err := rows.Scan(&id, &name, &sentCount, &failedCount, &totalEmails); err != nil {
			continue
		}

		// Check if this campaign has emails in the Redis queue
		queueLen := s.getCampaignQueueLength(id)
		pending := totalEmails - sentCount - failedCount

		// If queue is empty but there are pending emails, requeue
		if queueLen == 0 && pending > 0 {
			fmt.Printf("[AutoRequeue] Campaign %s (%s) has %d pending emails but empty queue - requeuing\n",
				id[:8], name, pending)

			// Set campaign status to running in Redis cache (in case it was lost)
			s.queue.SetCampaignStatus(id, "running")

			// Requeue pending emails
			count := s.requeuePendingEmails(id)
			fmt.Printf("[AutoRequeue] Campaign %s: requeued %d emails\n", id[:8], count)
		}
	}
}

// getCampaignQueueLength returns the number of emails in queue for a specific campaign
func (s *Server) getCampaignQueueLength(campaignID string) int64 {
	queueKey := fmt.Sprintf("smtpenviador:queue:campaign:%s", campaignID)
	length, err := s.queue.GetQueueLengthByKey(queueKey)
	if err != nil {
		return 0
	}
	return length
}

// checkAndStartScheduledCampaigns checks for scheduled campaigns and starts them when time comes
func (s *Server) checkAndStartScheduledCampaigns() {
	nowUnix := time.Now().Unix()
	rows, err := s.db.Query(`
		SELECT id, EXTRACT(EPOCH FROM scheduled_at)::bigint as scheduled_unix
		FROM campaigns
		WHERE status = 'scheduled'
		AND scheduled_at IS NOT NULL
	`)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		var scheduledUnix int64
		if err := rows.Scan(&id, &scheduledUnix); err != nil {
			continue
		}

		if scheduledUnix <= nowUnix {
			fmt.Printf("[Scheduler] Starting scheduled campaign %s\n", id)
			s.startScheduledCampaign(id)
		}
	}
}

// startScheduledCampaign starts a scheduled campaign
func (s *Server) startScheduledCampaign(id string) {
	// Get campaign details including user_id for SMTP isolation
	var listID, fromEmail, fromName, replyTo, subject, htmlContent, textContent, userID string
	var smtpIDsStr sql.NullString
	var trackOpens, trackClicks bool
	err := s.db.QueryRow(`
		SELECT list_id, COALESCE(smtp_ids, ''), from_email, from_name, reply_to, subject, html_content, text_content,
		       COALESCE(track_opens, true), COALESCE(track_clicks, true), user_id
		FROM campaigns WHERE id = $1 AND status = 'scheduled'
	`, id).Scan(&listID, &smtpIDsStr, &fromEmail, &fromName, &replyTo, &subject, &htmlContent, &textContent, &trackOpens, &trackClicks, &userID)

	if err != nil {
		fmt.Printf("[Scheduler] Error getting campaign %s: %v\n", id, err)
		return
	}

	// Parse SMTP IDs for user isolation
	var smtpIDs []string
	if smtpIDsStr.Valid && smtpIDsStr.String != "" {
		smtpIDs = strings.Split(smtpIDsStr.String, ",")
		for i := range smtpIDs {
			smtpIDs[i] = strings.TrimSpace(smtpIDs[i])
		}
	}

	// If no SMTPs specified, get ALL active SMTPs for the campaign owner (NEVER from other users)
	if len(smtpIDs) == 0 {
		smtpIDs = s.getUserActiveSMTPIDs(userID)
		if len(smtpIDs) == 0 {
			fmt.Printf("[Scheduler] Campaign %s has no active SMTPs for user %s - skipping\n", id, userID)
			return
		}
	}

	// Get tracking domain for campaign owner
	trackingDomain := s.getTrackingDomainForCampaign(id)

	// Queue emails - try ClickHouse first, then PostgreSQL
	listIDs := []string{listID}
	count := 0

	if s.ch != nil {
		ctx := context.Background()
		chEmails, err := s.ch.GetEmailsForCampaign(ctx, listIDs)
		if err != nil {
			fmt.Printf("[Scheduler] Error getting emails from ClickHouse: %v\n", err)
		} else {
			count = s.queueClickHouseEmails(chEmails, id, fromEmail, fromName, replyTo, subject, htmlContent, textContent, trackOpens, trackClicks, trackingDomain, 1, 0, smtpIDs)
		}
	}

	// Fallback to PostgreSQL
	if count == 0 {
		rows, err := s.db.Query(`
			SELECT id, email, name, custom1, custom2, custom3, custom4, custom5
			FROM emails
			WHERE list_id = $1 AND valid = true AND bounced = false AND unsubscribed = false
		`, listID)
		if err != nil {
			return
		}
		defer rows.Close()
		count = s.queueEmails(rows, id, fromEmail, fromName, replyTo, subject, htmlContent, textContent, trackOpens, trackClicks, trackingDomain, 1, 0, smtpIDs)
	}

	fmt.Printf("[Scheduler] Queued %d emails for campaign %s\n", count, id)

	// Update campaign status
	s.db.Exec(`
		UPDATE campaigns SET status = 'running', started_at = NOW(), total_emails = $1, scheduled_at = NULL
		WHERE id = $2
	`, count, id)
	fmt.Printf("[Scheduler] Campaign %s started!\n", id)
}

// setupMiddlewares configures middlewares
func (s *Server) setupMiddlewares() {
	s.app.Use(recover.New())
	s.app.Use(logger.New(logger.Config{
		Format: "[${time}] ${status} - ${method} ${path} (${latency})\n",
	}))
	s.app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,PUT,DELETE,OPTIONS",
		AllowHeaders: "Origin,Content-Type,Accept,Authorization",
	}))
}

// setupRoutes configures API routes
func (s *Server) setupRoutes() {
	// Health check
	s.app.Get("/health", s.healthCheck)

	// API v1
	api := s.app.Group("/api/v1")

	// Auth routes
	auth := api.Group("/auth")
	auth.Post("/login", s.login)
	auth.Post("/logout", s.logout)
	auth.Get("/reset-admin", s.resetAdmin) // Temporary: reset admin password

	// Protected routes
	protected := api.Group("/", s.authMiddleware)

	// Dashboard stats
	protected.Get("/stats", s.getStats)
	protected.Get("/stats/activity", s.getRecentActivity)
	protected.Get("/stats/tracking", s.getDashboardTrackingStats)
	protected.Get("/stats/combined", s.getCombinedStats)
	protected.Get("/stats/realtime", websocket.New(s.realtimeStats))

	// SMTP Servers
	smtp := protected.Group("/smtp")
	smtp.Get("/", s.listSMTPs)
	smtp.Post("/", s.createSMTP)
	smtp.Post("/test-connection", s.testSMTPConnectionPreview) // Test SMTP without saving
	smtp.Get("/:id", s.getSMTP)
	smtp.Put("/:id", s.updateSMTP)
	smtp.Delete("/:id", s.deleteSMTP)
	smtp.Post("/:id/test", s.testSMTP)
	smtp.Post("/:id/send-test", s.sendTestEmail)
	smtp.Post("/refresh", s.refreshSMTPs)
	// SMTP Senders
	smtp.Get("/:id/senders", s.listSMTPSenders)
	smtp.Post("/:id/senders", s.addSMTPSender)
	smtp.Post("/:id/senders/bulk", s.addSMTPSendersBulk)
	smtp.Delete("/:id/senders/:senderId", s.deleteSMTPSender)
	smtp.Post("/:id/senders/:senderId/toggle", s.toggleSMTPSender)

	// Email Lists
	lists := protected.Group("/lists")
	lists.Get("/", s.listEmailLists)
	lists.Post("/", s.createEmailList)
	lists.Get("/:id", s.getEmailList)
	lists.Put("/:id", s.updateEmailList)
	lists.Delete("/:id", s.deleteEmailList)
	lists.Delete("/:id/force", s.forceDeleteEmailList)
	lists.Post("/:id/upload", s.uploadEmails)
	lists.Post("/:id/upload-async", s.uploadEmailsAsync)
	lists.Post("/upload-split", s.uploadEmailsSplit)
	lists.Get("/:id/import-jobs", s.getListImportJobs)
	lists.Get("/:id/emails", s.getListEmails)
	lists.Post("/:id/emails", s.addEmailManually)
	lists.Put("/:id/emails/:emailId", s.updateEmail)
	lists.Delete("/:id/emails/:emailId", s.deleteEmail)
	lists.Get("/:id/download", s.downloadList)
	lists.Put("/:id/group", s.moveListToGroup)
	lists.Post("/refresh-counts", s.refreshListCounts)

	// Email List Groups
	groups := protected.Group("/groups")
	groups.Get("/", s.listGroups)
	groups.Post("/", s.createGroup)
	groups.Put("/:id", s.updateGroup)
	groups.Delete("/:id", s.deleteGroup)

	// Import job status and control (outside of lists group for simpler access)
	protected.Get("/import-status/:jobId", s.getImportStatus)
	protected.Post("/import-cancel/:jobId", s.cancelImportJob)

	// Templates
	templates := protected.Group("/templates")
	templates.Get("/", s.listTemplates)
	templates.Post("/", s.createTemplate)
	templates.Get("/:id", s.getTemplate)
	templates.Put("/:id", s.updateTemplate)
	templates.Delete("/:id", s.deleteTemplate)

	// Campaigns
	campaigns := protected.Group("/campaigns")
	campaigns.Get("/", s.listCampaigns)
	campaigns.Post("/", s.createCampaign)
	campaigns.Get("/:id", s.getCampaign)
	campaigns.Put("/:id", s.updateCampaign)
	campaigns.Delete("/:id", s.deleteCampaign)
	campaigns.Post("/:id/start", s.startCampaign)
	campaigns.Post("/:id/pause", s.pauseCampaign)
	campaigns.Post("/:id/resume", s.resumeCampaign)
	campaigns.Post("/:id/requeue", s.requeueCampaign)
	campaigns.Post("/:id/cancel", s.cancelCampaign)
	campaigns.Post("/:id/cancel-auto-start", s.cancelAutoStart)
	campaigns.Get("/:id/stats", s.getCampaignStats)
	campaigns.Post("/:id/clone", s.cloneCampaign)
	campaigns.Post("/:id/schedule", s.scheduleCampaign)
	campaigns.Post("/:id/cancel-schedule", s.cancelSchedule)
	campaigns.Post("/:id/resend", s.resendCampaign)
	campaigns.Post("/:id/resend-failed", s.resendToFailed)
	campaigns.Post("/:id/resend-non-openers", s.resendToNonOpeners)
	campaigns.Get("/:id/export", s.exportCampaignCSV)
	campaigns.Get("/:id/export-emails", s.exportCampaignEmails)
	campaigns.Get("/:id/details", s.getCampaignDetails)
	campaigns.Get("/:id/tracking-stats", s.getTrackingStats)
	campaigns.Get("/:id/tracking-stats/cities/:countryCode", s.getTrackingCityStats)

	// Blacklist
	blacklist := protected.Group("/blacklist")
	blacklist.Get("/", s.listBlacklist)
	blacklist.Post("/", s.addToBlacklist)
	blacklist.Delete("/:id", s.removeFromBlacklist)
	blacklist.Delete("/", s.clearBlacklist)
	blacklist.Post("/import", s.importBlacklist)
	// SQL file migration for blacklist (async)
	blacklist.Get("/migration/info", s.getMigrationStats)
	blacklist.Get("/migration/jobs", s.listBlacklistImportJobs)
	blacklist.Post("/migration/import", s.startBlacklistImport)
	blacklist.Get("/migration/status/:jobId", s.getBlacklistImportStatus)

	// Tracking endpoints (public)
	s.app.Get("/track/open/:campaignId/:emailId", s.trackOpen)
	s.app.Get("/track/click/:campaignId/:emailId", s.trackClick)
	s.app.Get("/unsubscribe/:campaignId/:emailId", s.unsubscribe)

	// Logs
	protected.Get("/logs", s.getLogs)

	// Settings
	settings := protected.Group("/settings")
	settings.Get("/", s.getSettings)
	settings.Get("/server-info", s.getServerInfo)
	settings.Post("/restart", s.restartServer)
	settings.Post("/test-proxy", s.testProxy)
	settings.Get("/:key", s.getSetting)
	settings.Put("/:key", s.updateSetting)
	settings.Put("/", s.updateSettings)

	// User Management
	users := protected.Group("/users")
	users.Get("/", s.listUsers)
	users.Get("/me", s.getCurrentUser)
	users.Post("/", s.createUser)
	users.Get("/:id", s.getUser)
	users.Put("/:id", s.updateUser)
	users.Delete("/:id", s.deleteUser)
	users.Put("/:id/password", s.changePassword)

	// Warmup System
	warmup := protected.Group("/warmup")
	warmup.Get("/stats", s.getWarmupStats)
	warmup.Get("/activity", s.getWarmupActivity)
	warmup.Get("/diagnostic", s.getWarmupDiagnostic)
	warmup.Post("/trigger", s.triggerWarmup)
	// Warmup SMTPs
	warmup.Get("/smtps", s.listWarmupSMTPs)
	warmup.Post("/smtps", s.createWarmupSMTP)
	warmup.Put("/smtps/:id", s.updateWarmupSMTP)
	warmup.Delete("/smtps/:id", s.deleteWarmupSMTP)
	warmup.Post("/smtps/:id/toggle", s.toggleWarmupSMTP)
	warmup.Post("/smtps/:id/trigger", s.triggerWarmupSMTP)
	warmup.Get("/smtps/:id/stats", s.getWarmupSMTPStats)
	warmup.Put("/smtps/:id/schedule", s.updateWarmupSchedule)
	warmup.Post("/smtps/:id/internal-warmup", s.toggleInternalWarmup)
	// Warmup Seeds
	warmup.Get("/seeds", s.listWarmupSeeds)
	warmup.Post("/seeds", s.createWarmupSeed)
	warmup.Post("/seeds/test-connection", s.testSeedConnectionPreview) // Test before saving
	warmup.Post("/seeds/verify-all", s.verifyAllSeeds)                 // Verify all seeds
	warmup.Delete("/seeds/with-errors", s.deleteErrorSeeds)            // Delete all error seeds
	warmup.Post("/seeds/fix-orphaned", s.fixOrphanedSeeds)             // Fix seeds with NULL user_id
	warmup.Post("/seeds/batch-pause", s.batchPauseSeeds)               // Pause selected seeds
	warmup.Post("/seeds/batch-activate", s.batchActivateSeeds)         // Activate selected seeds
	warmup.Post("/seeds/batch-delete", s.batchDeleteSeeds)             // Delete selected seeds
	warmup.Put("/seeds/:id", s.updateWarmupSeed)
	warmup.Delete("/seeds/:id", s.deleteWarmupSeed)
	warmup.Post("/seeds/:id/test", s.testWarmupSeed)
	warmup.Post("/seeds/:id/clean", s.cleanSeedEmails) // Clean all emails from seed
	warmup.Post("/seeds/:id/toggle", s.toggleWarmupSeed)
	warmup.Post("/seeds/:id/trigger", s.triggerSeedSend)
	warmup.Post("/check-imap", s.triggerIMAPCheck)
	// Warmup Templates
	warmup.Get("/templates", s.listWarmupTemplates)
	warmup.Post("/templates", s.createWarmupTemplate)
	warmup.Delete("/templates/:id", s.deleteWarmupTemplate)
	// Sender IMAP (for internal warmup)
	warmup.Post("/senders/:id/imap", s.updateSenderIMAP)
	warmup.Post("/senders/:id/test-imap", s.testSenderIMAP)
	// Warmup Settings
	warmup.Get("/settings", s.getWarmupSettings)
	warmup.Put("/settings", s.updateWarmupSettings)

	// Web Warmup (browser automation)
	s.registerWebWarmupRoutes(protected)
}

// Start starts the API server
func (s *Server) Start() error {
	return s.app.Listen(":" + s.cfg.APIPort)
}

// Stop stops the API server
func (s *Server) Stop() error {
	return s.app.Shutdown()
}

// healthCheck returns server health status
func (s *Server) healthCheck(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status":  "ok",
		"version": "1.0.0",
		"engine":  s.engine.IsRunning(),
	})
}
