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
	w.stats.TotalSent.Add(1)
	w.queue.IncrementStat("sent", 1)
	w.updateEmailStatus(job.ID, "sent", "")
	w.updateSMTPStats(smtp.ID, true)
	w.updateCampaignSentCount(job.CampaignID)

	// Log every email sent for monitoring
	sent := w.stats.TotalSent.Load()
	log.Printf("✅ [%d] Email enviado para %s via %s", sent, job.To, smtp.Name)
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

	// Try to get an available SMTP (with rate limit capacity)
	var smtp *SMTPConnection
	var sender *SMTPSender
	maxAttempts := w.smtpPool.GetActiveCount()
	if maxAttempts == 0 {
		maxAttempts = 1
	}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		smtp = w.smtpPool.GetNextSMTP()
		if smtp == nil {
			break
		}

		// Check rate limits for THIS queue type (campaign/warmup have separate limits)
		if !w.queue.CheckBothRateLimitsForType(smtp.ID, smtp.MaxPerMinute, smtp.MaxPerHour, queueType) {
			smtp = nil // Try another SMTP
			continue
		}

		// Get sender for this SMTP
		sender = smtp.GetNextSender()
		if sender == nil {
			smtp = nil
			continue
		}

		// Found a valid SMTP with capacity and sender
		break
	}

	// No SMTP available with capacity
	if smtp == nil {
		if job.Retries == 0 {
			log.Printf("⚠️ Worker %d: No SMTP with capacity for %s, will retry", w.id, job.To)
		}
		job.Retries++
		w.pushToQueue(job, queueType)
		time.Sleep(1 * time.Second) // Wait 1s before retrying
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
	w.stats.TotalSent.Add(1)
	w.queue.IncrementStat("sent", 1)
	w.updateEmailStatus(job.ID, "sent", "")
	w.updateSMTPStats(smtp.ID, true)
	w.updateCampaignSentCount(job.CampaignID)

	sent := w.stats.TotalSent.Load()
	prefix := "📧"
	if queueType == "warmup" {
		prefix = "🔥"
	}
	log.Printf("%s [%d] Email enviado para %s via %s", prefix, sent, job.To, smtp.Name)
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

// updateCampaignSentCount updates the campaign sent_count
func (w *Worker) updateCampaignSentCount(campaignID string) {
	w.db.Exec(`UPDATE campaigns SET sent_count = sent_count + 1 WHERE id = $1`, campaignID)
	w.checkCampaignCompletion(campaignID)
}

// updateCampaignFailedCount updates the campaign failed_count
func (w *Worker) updateCampaignFailedCount(campaignID string) {
	w.db.Exec(`UPDATE campaigns SET failed_count = failed_count + 1 WHERE id = $1`, campaignID)
	w.checkCampaignCompletion(campaignID)
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
	log.Printf("[Worker %d] Generated tracking pixel: %s", w.id, pixelURL)
	return `<img src="` + pixelURL + `" width="1" height="1" style="display:none" />`
}

// processLinks replaces links with tracking URLs
func (w *Worker) processLinks(trackingDomain, content, campaignID, emailID string) string {
	trackBase := trackingDomain + "/track/click/" + campaignID + "/" + emailID + "?url="
	log.Printf("[Worker %d] Processing links with tracking base: %s", w.id, trackBase)

	// Use regex to find and replace href URLs properly
	// This captures the full URL including any query parameters
	linkRegex := regexp.MustCompile(`href="(https?://[^"]+)"`)

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

// updateEmailStatus updates the campaign_email status
func (w *Worker) updateEmailStatus(id, status, errorMsg string) {
	var query string
	var err error
	var result sql.Result

	if status == "sent" {
		query = `UPDATE campaign_emails SET status = $1, error_message = $2, sent_at = NOW() WHERE id = $3`
	} else {
		query = `UPDATE campaign_emails SET status = $1, error_message = $2 WHERE id = $3`
	}

	result, err = w.db.Exec(query, status, errorMsg, id)
	if err != nil {
		log.Printf("❌ Worker %d: Failed to update email status for %s: %v", w.id, id, err)
		return
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		log.Printf("⚠️ Worker %d: No rows updated for email %s (status: %s) - record may not exist", w.id, id, status)
	} else {
		log.Printf("📝 Worker %d: Updated email %s status to %s", w.id, id, status)
	}
}

// updateSMTPStats updates SMTP server statistics
func (w *Worker) updateSMTPStats(smtpID string, success bool) {
	if success {
		w.db.Exec(`UPDATE smtp_servers SET total_sent = total_sent + 1 WHERE id = $1`, smtpID)
	} else {
		w.db.Exec(`UPDATE smtp_servers SET total_failed = total_failed + 1 WHERE id = $1`, smtpID)
	}
}
