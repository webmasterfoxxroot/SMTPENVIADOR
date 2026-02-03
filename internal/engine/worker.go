package engine

import (
	"database/sql"
	"log"
	"net/url"
	"regexp"
	"strings"
	"time"

	"smtpenviador/internal/config"
	"smtpenviador/internal/queue"
)

// Pre-compiled regex for link processing (compiled once, reused for all emails)
var linkRegex = regexp.MustCompile(`href="(https?://[^"]+)"`)

// Worker processes email jobs from the queue
type Worker struct {
	id       int
	cfg      *config.Config
	db       *sql.DB
	queue    *queue.Manager
	smtpPool *SMTPPool
	stats    *EngineStats
}

// NewWorker creates a new worker
func NewWorker(id int, cfg *config.Config, db *sql.DB, q *queue.Manager, pool *SMTPPool, stats *EngineStats) *Worker {
	return &Worker{
		id:       id,
		cfg:      cfg,
		db:       db,
		queue:    q,
		smtpPool: pool,
		stats:    stats,
	}
}

// Start starts the worker loop (legacy - uses all queues)
func (w *Worker) Start(stopChan chan struct{}) {
	log.Printf("👷 Worker %d started (legacy)", w.id)
	w.stats.ActiveWorkers.Add(1)
	defer w.stats.ActiveWorkers.Add(-1)

	for {
		select {
		case <-stopChan:
			log.Printf("👷 Worker %d stopping", w.id)
			return
		default:
			w.processJob()
		}
	}
}

// StartCampaign starts a dedicated campaign worker (uses campaign queue only)
func (w *Worker) StartCampaign(stopChan chan struct{}) {
	log.Printf("📧 Campaign Worker %d started", w.id)
	w.stats.ActiveWorkers.Add(1)
	defer w.stats.ActiveWorkers.Add(-1)

	for {
		select {
		case <-stopChan:
			log.Printf("📧 Campaign Worker %d stopping", w.id)
			return
		default:
			w.processCampaignJob()
		}
	}
}

// StartWarmup starts a dedicated warmup worker (uses warmup queue only)
func (w *Worker) StartWarmup(stopChan chan struct{}) {
	log.Printf("🔥 Warmup Worker %d started", w.id)
	w.stats.ActiveWorkers.Add(1)
	defer w.stats.ActiveWorkers.Add(-1)

	for {
		select {
		case <-stopChan:
			log.Printf("🔥 Warmup Worker %d stopping", w.id)
			return
		default:
			w.processWarmupJob()
		}
	}
}

// processJob gets and processes a single job
func (w *Worker) processJob() {
	// Get job from queue
	job, err := w.queue.Pop()
	if err != nil {
		log.Printf("⚠️ Worker %d: Failed to pop job: %v", w.id, err)
		time.Sleep(500 * time.Millisecond)
		return
	}

	// No job available, wait a bit
	if job == nil {
		time.Sleep(100 * time.Millisecond)
		return
	}

	// Check job age - if job is too old and has been retried many times, skip it
	if job.Retries > 10 {
		log.Printf("⚠️ Worker %d: Job for %s exceeded max retries (%d), marking as failed", w.id, job.To, job.Retries)
		w.queue.PushFailed(job, "Max retries exceeded")
		w.updateEmailStatus(job.ID, "failed", "Max retries exceeded")
		w.updateCampaignFailedCount(job.CampaignID)
		return
	}

	// Get SMTP connection
	smtp := w.smtpPool.GetNextSMTP()
	if smtp == nil {
		// No SMTPs available - wait longer before pushing back
		if job.Retries == 0 {
			log.Printf("⚠️ Worker %d: No SMTP available for %s, will retry", w.id, job.To)
		}
		job.Retries++
		w.queue.Push(job)
		time.Sleep(5 * time.Second) // Wait 5 seconds before retrying
		return
	}

	// Get next sender from SMTP (rotates automatically)
	sender := smtp.GetNextSender()
	if sender == nil {
		if job.Retries == 0 {
			log.Printf("⚠️ Worker %d: No senders for SMTP %s", w.id, smtp.Name)
		}
		job.Retries++
		w.queue.Push(job)
		time.Sleep(2 * time.Second)
		return
	}

	// Check rate limit
	if !w.queue.CheckRateLimit(smtp.ID, smtp.MaxPerMinute) {
		// Rate limited, push back without incrementing retries (this is normal)
		w.queue.Push(job)
		time.Sleep(100 * time.Millisecond)
		return
	}

	// Check campaign batch limit (for slow sending)
	if job.BatchInterval > 0 && !w.queue.CheckCampaignBatchLimit(job.CampaignID, job.BatchSize, job.BatchInterval) {
		// Batch limit reached, push back and wait
		w.queue.Push(job)
		time.Sleep(500 * time.Millisecond)
		return
	}

	// Process variables in content
	htmlContent := w.processVariables(job.HTMLContent, job.Variables, job.To, job.ToName)
	textContent := w.processVariables(job.TextContent, job.Variables, job.To, job.ToName)
	subject := w.processVariables(job.Subject, job.Variables, job.To, job.ToName)

	// Get tracking domain (from job or fallback to config)
	trackingDomain := job.TrackingDomain
	if trackingDomain == "" {
		trackingDomain = w.cfg.TrackingDomain
	}

	// Add tracking pixel for open tracking
	if job.TrackOpens && trackingDomain != "" {
		trackingPixel := w.generateTrackingPixel(trackingDomain, job.CampaignID, job.EmailID)
		htmlContent = strings.Replace(htmlContent, "</body>", trackingPixel+"</body>", 1)
	}

	// Process links for click tracking
	if job.TrackClicks && trackingDomain != "" {
		htmlContent = w.processLinks(trackingDomain, htmlContent, job.CampaignID, job.EmailID)
	}

	// Use email from SMTP sender, but name from campaign
	fromEmail := sender.Email
	fromName := job.FromName // Name comes from campaign, not SMTP sender
	replyTo := sender.ReplyTo
	if replyTo == "" {
		replyTo = job.ReplyTo // Fallback to campaign's reply_to if sender doesn't have one
	}

	// Send email
	err = smtp.Send(SendParams{
		From:        fromEmail,
		FromName:    fromName,
		To:          job.To,
		ToName:      job.ToName,
		ReplyTo:     replyTo,
		Subject:     subject,
		HTMLContent: htmlContent,
		TextContent: textContent,
	})

	if err != nil {
		w.stats.TotalFailed.Add(1)
		w.queue.IncrementStat("failed", 1)

		// Update campaign_emails status
		w.updateEmailStatus(job.ID, "failed", err.Error())

		// Retry logic
		if job.Retries < 3 {
			job.Retries++
			log.Printf("🔄 Worker %d: Retry %d for %s", w.id, job.Retries, job.To)
			w.queue.Push(job)
		} else {
			w.queue.PushFailed(job, err.Error())
			w.updateCampaignFailedCount(job.CampaignID)
		}

		log.Printf("❌ Worker %d: Failed to send to %s: %v", w.id, job.To, err)
		return
	}

	// Success - increment rate limit AFTER successful send
	w.queue.IncrementRateLimit(smtp.ID)
	// Increment campaign batch counter for slow sending
	if job.BatchInterval > 0 {
		w.queue.IncrementCampaignBatchCount(job.CampaignID, job.BatchInterval)
	}
	w.stats.TotalSent.Add(1)
	w.queue.IncrementStat("sent", 1)
	w.updateEmailStatus(job.ID, "sent", "")
	w.updateSMTPStats(smtp.ID, true)
	w.updateCampaignSentCount(job.CampaignID)

	// Log every 100 emails for monitoring (reduces log overhead)
	sent := w.stats.TotalSent.Load()
	if sent%100 == 0 {
		log.Printf("✅ [%d] emails enviados (último: %s via %s)", sent, job.To, smtp.Name)
	}
}

// processCampaignJob processes jobs from the CAMPAIGN queue only
func (w *Worker) processCampaignJob() {
	// Get job from CAMPAIGN queue only (BRPOP waits for jobs efficiently)
	job, err := w.queue.PopCampaign()
	if err != nil {
		log.Printf("⚠️ Campaign Worker %d: Failed to pop job: %v", w.id, err)
		time.Sleep(500 * time.Millisecond)
		return
	}

	// No job available (BRPOP timed out), continue to next iteration
	if job == nil {
		return
	}

	// Process the job (same logic as processJob but uses campaign queue for retries)
	w.processJobWithQueue(job, "campaign")
}

// processWarmupJob processes jobs from the WARMUP queue only
func (w *Worker) processWarmupJob() {
	// Get job from WARMUP queue only (BRPOP waits for jobs efficiently)
	job, err := w.queue.PopWarmup()
	if err != nil {
		log.Printf("⚠️ Warmup Worker %d: Failed to pop job: %v", w.id, err)
		time.Sleep(500 * time.Millisecond)
		return
	}

	// No job available (BRPOP timed out), continue to next iteration
	if job == nil {
		return
	}

	// Process the job (same logic as processJob but uses warmup queue for retries)
	w.processJobWithQueue(job, "warmup")
}

// processJobWithQueue processes a job using the specified queue for retries
func (w *Worker) processJobWithQueue(job *queue.EmailJob, queueType string) {
	// Check job age - if job is too old and has been retried many times, skip it
	if job.Retries > 10 {
		log.Printf("⚠️ Worker %d: Job for %s exceeded max retries (%d), marking as failed", w.id, job.To, job.Retries)
		w.queue.PushFailed(job, "Max retries exceeded")
		w.updateEmailStatus(job.ID, "failed", "Max retries exceeded")
		w.updateCampaignFailedCount(job.CampaignID)
		return
	}

	// CRITICAL: Job MUST have SMTPIDs for user isolation
	// If empty, fail the job - this prevents using SMTPs from other users
	if len(job.SMTPIDs) == 0 {
		log.Printf("❌ Worker %d: Job for %s has no SMTPIDs - failing to prevent cross-user SMTP usage", w.id, job.To)
		w.queue.PushFailed(job, "No SMTPIDs specified - user isolation error")
		w.updateEmailStatus(job.ID, "failed", "Campanha sem SMTPs configurados")
		w.updateCampaignFailedCount(job.CampaignID)
		return
	}

	// Build allowed SMTP ID map for quick lookup (user isolation)
	allowedSMTPIDs := make(map[string]bool)
	for _, id := range job.SMTPIDs {
		allowedSMTPIDs[id] = true
	}

	// Get all active SMTPs and find the one with most remaining capacity
	var smtp *SMTPConnection
	var sender *SMTPSender
	allSMTPs := w.smtpPool.GetAllActiveSMTPs()

	if len(allSMTPs) > 0 {
		var bestCapacity int64 = -1

		for _, candidate := range allSMTPs {
			// User isolation: ONLY use SMTPs specified in the job (MANDATORY)
			if !allowedSMTPIDs[candidate.ID] {
				continue // Skip - not allowed for this user/campaign
			}

			// Check rate limits for THIS queue type (campaign/warmup have separate limits)
			if !w.queue.CheckBothRateLimitsForType(candidate.ID, candidate.MaxPerMinute, candidate.MaxPerHour, queueType) {
				continue // Skip - no capacity
			}

			// Get remaining capacity for this SMTP
			capacity := w.queue.GetSMTPRemainingCapacity(candidate.ID, candidate.MaxPerMinute, queueType)

			// Choose SMTP with most remaining capacity
			if capacity > bestCapacity {
				// Get sender for this SMTP
				candidateSender := candidate.GetNextSender()
				if candidateSender == nil {
					continue
				}
				bestCapacity = capacity
				smtp = candidate
				sender = candidateSender
			}
		}
	}

	// No SMTP available with capacity
	if smtp == nil {
		if job.Retries == 0 {
			log.Printf("⚠️ Worker %d: No SMTP with capacity for %s, will retry", w.id, job.To)
		}
		job.Retries++
		w.pushToQueue(job, queueType)
		// Wait until next minute boundary when per-minute rate limits reset
		// This prevents busy-looping when all SMTPs are at capacity
		now := time.Now()
		nextMinute := now.Truncate(time.Minute).Add(time.Minute)
		waitTime := nextMinute.Sub(now)
		// Cap wait time at 10 seconds - balances efficiency with responsiveness
		if waitTime > 10*time.Second {
			waitTime = 10 * time.Second
		}
		// Minimum wait of 1 second to avoid busy-looping
		if waitTime < 1*time.Second {
			waitTime = 1 * time.Second
		}
		time.Sleep(waitTime)
		return
	}

	// Check campaign batch limit (for slow sending)
	if job.BatchInterval > 0 && !w.queue.CheckCampaignBatchLimit(job.CampaignID, job.BatchSize, job.BatchInterval) {
		// Batch limit reached, push back and wait
		w.pushToQueue(job, queueType)
		time.Sleep(500 * time.Millisecond)
		return
	}

	// Process variables in content
	htmlContent := w.processVariables(job.HTMLContent, job.Variables, job.To, job.ToName)
	textContent := w.processVariables(job.TextContent, job.Variables, job.To, job.ToName)
	subject := w.processVariables(job.Subject, job.Variables, job.To, job.ToName)

	// Get tracking domain (from job or fallback to config)
	trackingDomain := job.TrackingDomain
	if trackingDomain == "" {
		trackingDomain = w.cfg.TrackingDomain
	}

	// Add tracking pixel for open tracking
	if job.TrackOpens && trackingDomain != "" {
		trackingPixel := w.generateTrackingPixel(trackingDomain, job.CampaignID, job.EmailID)
		htmlContent = strings.Replace(htmlContent, "</body>", trackingPixel+"</body>", 1)
	}

	// Process links for click tracking
	if job.TrackClicks && trackingDomain != "" {
		htmlContent = w.processLinks(trackingDomain, htmlContent, job.CampaignID, job.EmailID)
	}

	// Use email from SMTP sender, but name from campaign
	fromEmail := sender.Email
	fromName := job.FromName
	replyTo := sender.ReplyTo
	if replyTo == "" {
		replyTo = job.ReplyTo
	}

	// Send email
	err := smtp.Send(SendParams{
		From:        fromEmail,
		FromName:    fromName,
		To:          job.To,
		ToName:      job.ToName,
		ReplyTo:     replyTo,
		Subject:     subject,
		HTMLContent: htmlContent,
		TextContent: textContent,
	})

	if err != nil {
		w.stats.TotalFailed.Add(1)
		w.queue.IncrementStat("failed", 1)
		w.updateEmailStatus(job.ID, "failed", err.Error())

		// Retry logic
		if job.Retries < 3 {
			job.Retries++
			log.Printf("🔄 Worker %d: Retry %d for %s", w.id, job.Retries, job.To)
			w.pushToQueue(job, queueType)
		} else {
			w.queue.PushFailed(job, err.Error())
			w.updateCampaignFailedCount(job.CampaignID)
		}

		log.Printf("❌ Worker %d: Failed to send to %s: %v", w.id, job.To, err)
		return
	}

	// Success - increment rate limit for THIS queue type AFTER successful send
	w.queue.IncrementRateLimitForType(smtp.ID, queueType)
	// Increment campaign batch counter for slow sending
	if job.BatchInterval > 0 {
		w.queue.IncrementCampaignBatchCount(job.CampaignID, job.BatchInterval)
	}
	w.stats.TotalSent.Add(1)
	w.queue.IncrementStat("sent", 1)
	w.updateEmailStatus(job.ID, "sent", "")
	w.updateSMTPStats(smtp.ID, true)
	w.updateCampaignSentCount(job.CampaignID)

	// Log every 100 emails for monitoring (reduces log overhead)
	sent := w.stats.TotalSent.Load()
	if sent%100 == 0 {
		prefix := "📧"
		if queueType == "warmup" {
			prefix = "🔥"
		}
		log.Printf("%s [%d] emails enviados (último: %s via %s)", prefix, sent, job.To, smtp.Name)
	}
}

// pushToQueue pushes a job back to the appropriate queue
func (w *Worker) pushToQueue(job *queue.EmailJob, queueType string) {
	switch queueType {
	case "campaign":
		w.queue.PushCampaign(job)
	case "warmup":
		w.queue.PushWarmup(job)
	default:
		w.queue.Push(job)
	}
}

// updateCampaignSentCount updates the campaign sent_count (async to avoid blocking)
func (w *Worker) updateCampaignSentCount(campaignID string) {
	go func() {
		w.db.Exec(`UPDATE campaigns SET sent_count = sent_count + 1 WHERE id = $1`, campaignID)
		w.checkCampaignCompletion(campaignID)
	}()
}

// updateCampaignFailedCount updates the campaign failed_count (async to avoid blocking)
func (w *Worker) updateCampaignFailedCount(campaignID string) {
	go func() {
		w.db.Exec(`UPDATE campaigns SET failed_count = failed_count + 1 WHERE id = $1`, campaignID)
		w.checkCampaignCompletion(campaignID)
	}()
}

// checkCampaignCompletion checks if campaign is complete and updates status
func (w *Worker) checkCampaignCompletion(campaignID string) {
	var totalEmails, sentCount, failedCount int
	var status string

	err := w.db.QueryRow(`
		SELECT total_emails, sent_count, failed_count, status
		FROM campaigns WHERE id = $1
	`, campaignID).Scan(&totalEmails, &sentCount, &failedCount, &status)

	if err != nil {
		return
	}

	// Only update if still running and all emails processed
	if status == "running" && (sentCount+failedCount) >= totalEmails {
		w.db.Exec(`
			UPDATE campaigns
			SET status = 'completed', completed_at = NOW()
			WHERE id = $1
		`, campaignID)
		log.Printf("🏁 Campaign %s completed: %d sent, %d failed", campaignID, sentCount, failedCount)
	}
}

// processVariables replaces variables in content
func (w *Worker) processVariables(content string, variables map[string]string, email, name string) string {
	// Built-in variables
	content = strings.ReplaceAll(content, "{{email}}", email)
	content = strings.ReplaceAll(content, "{{nome}}", name)
	content = strings.ReplaceAll(content, "{{name}}", name)
	content = strings.ReplaceAll(content, "{{data}}", time.Now().Format("02/01/2006"))
	content = strings.ReplaceAll(content, "{{date}}", time.Now().Format("2006-01-02"))

	// Custom variables
	for key, value := range variables {
		content = strings.ReplaceAll(content, "{{"+key+"}}", value)
	}

	return content
}

// generateTrackingPixel generates a tracking pixel for opens
func (w *Worker) generateTrackingPixel(trackingDomain, campaignID, emailID string) string {
	pixelURL := trackingDomain + `/track/open/` + campaignID + `/` + emailID
	return `<img src="` + pixelURL + `" width="1" height="1" style="display:none" />`
}

// processLinks replaces links with tracking URLs (uses pre-compiled regex for speed)
func (w *Worker) processLinks(trackingDomain, content, campaignID, emailID string) string {
	trackBase := trackingDomain + "/track/click/" + campaignID + "/" + emailID + "?url="

	// Use pre-compiled regex (package level) for speed
	content = linkRegex.ReplaceAllStringFunc(content, func(match string) string {
		// Extract the URL from href="URL"
		urlMatch := linkRegex.FindStringSubmatch(match)
		if len(urlMatch) < 2 {
			return match
		}
		originalURL := urlMatch[1]

		// Skip tracking links (don't double-track)
		if strings.Contains(originalURL, "/track/") {
			return match
		}

		// Skip unsubscribe links
		if strings.Contains(originalURL, "/unsubscribe/") {
			return match
		}

		// URL-encode the original URL so it's safely passed as a query parameter
		encodedURL := url.QueryEscape(originalURL)
		return `href="` + trackBase + encodedURL + `"`
	})

	return content
}

// updateEmailStatus updates the campaign_email status (async to avoid blocking)
func (w *Worker) updateEmailStatus(id, status, errorMsg string) {
	go func() {
		var query string

		if status == "sent" {
			query = `UPDATE campaign_emails SET status = $1, error_message = $2, sent_at = NOW() WHERE id = $3`
		} else {
			query = `UPDATE campaign_emails SET status = $1, error_message = $2 WHERE id = $3`
		}

		w.db.Exec(query, status, errorMsg, id)
	}()
}

// updateSMTPStats updates SMTP server statistics (async to avoid blocking)
func (w *Worker) updateSMTPStats(smtpID string, success bool) {
	go func() {
		if success {
			w.db.Exec(`UPDATE smtp_servers SET total_sent = total_sent + 1 WHERE id = $1`, smtpID)
		} else {
			w.db.Exec(`UPDATE smtp_servers SET total_failed = total_failed + 1 WHERE id = $1`, smtpID)
		}
	}()
}
