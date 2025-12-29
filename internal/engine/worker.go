package engine

import (
	"database/sql"
	"log"
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

// Start starts the worker loop
func (w *Worker) Start(stopChan chan struct{}) {
	log.Printf("👷 Worker %d started", w.id)
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

// processJob gets and processes a single job
func (w *Worker) processJob() {
	// Get job from queue
	job, err := w.queue.Pop()
	if err != nil {
		log.Printf("⚠️ Worker %d: Failed to pop job: %v", w.id, err)
		time.Sleep(100 * time.Millisecond)
		return
	}

	// No job available, wait a bit
	if job == nil {
		time.Sleep(50 * time.Millisecond)
		return
	}

	// Get SMTP connection
	smtp := w.smtpPool.GetNextSMTP()
	if smtp == nil {
		// No SMTPs available, push job back and wait
		w.queue.Push(job)
		time.Sleep(1 * time.Second)
		return
	}

	// Check rate limit
	if !w.queue.CheckRateLimit(smtp.ID, smtp.MaxPerMinute) {
		// Rate limited, try another SMTP or wait
		w.queue.Push(job)
		time.Sleep(100 * time.Millisecond)
		return
	}

	// Process variables in content
	htmlContent := w.processVariables(job.HTMLContent, job.Variables, job.To, job.ToName)
	textContent := w.processVariables(job.TextContent, job.Variables, job.To, job.ToName)
	subject := w.processVariables(job.Subject, job.Variables, job.To, job.ToName)

	// Add tracking pixel
	trackingPixel := w.generateTrackingPixel(job.CampaignID, job.EmailID)
	htmlContent = strings.Replace(htmlContent, "</body>", trackingPixel+"</body>", 1)

	// Process links for click tracking
	htmlContent = w.processLinks(htmlContent, job.CampaignID, job.EmailID)

	// Send email
	err = smtp.Send(SendParams{
		From:        job.From,
		FromName:    job.FromName,
		To:          job.To,
		ToName:      job.ToName,
		ReplyTo:     job.ReplyTo,
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
			w.queue.Push(job)
		} else {
			w.queue.PushFailed(job, err.Error())
		}

		log.Printf("❌ Worker %d: Failed to send to %s: %v", w.id, job.To, err)
		return
	}

	// Success
	w.stats.TotalSent.Add(1)
	w.queue.IncrementStat("sent", 1)
	w.updateEmailStatus(job.ID, "sent", "")
	w.updateSMTPStats(smtp.ID, true)
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
func (w *Worker) generateTrackingPixel(campaignID, emailID string) string {
	return `<img src="` + w.cfg.TrackingDomain + `/track/open/` + campaignID + `/` + emailID + `" width="1" height="1" style="display:none" />`
}

// processLinks replaces links with tracking URLs
func (w *Worker) processLinks(content, campaignID, emailID string) string {
	// Simple link replacement - in production use proper HTML parsing
	// This replaces href="http..." with tracking URLs
	trackBase := w.cfg.TrackingDomain + "/track/click/" + campaignID + "/" + emailID + "?url="

	// Replace http:// links
	content = strings.ReplaceAll(content, `href="http://`, `href="`+trackBase+`http://`)
	content = strings.ReplaceAll(content, `href="https://`, `href="`+trackBase+`https://`)

	return content
}

// updateEmailStatus updates the campaign_email status
func (w *Worker) updateEmailStatus(id, status, errorMsg string) {
	query := `UPDATE campaign_emails SET status = $1, error_message = $2, sent_at = CASE WHEN $1 = 'sent' THEN NOW() ELSE sent_at END WHERE id = $3`
	w.db.Exec(query, status, errorMsg, id)
}

// updateSMTPStats updates SMTP server statistics
func (w *Worker) updateSMTPStats(smtpID string, success bool) {
	if success {
		w.db.Exec(`UPDATE smtp_servers SET total_sent = total_sent + 1 WHERE id = $1`, smtpID)
	} else {
		w.db.Exec(`UPDATE smtp_servers SET total_failed = total_failed + 1 WHERE id = $1`, smtpID)
	}
}
