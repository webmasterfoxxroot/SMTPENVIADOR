package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"smtpenviador/internal/config"
)

const (
	// Campaign queue (high priority)
	QueueCampaign         = "smtpenviador:queue:campaign"
	QueueCampaignPriority = "smtpenviador:queue:campaign:priority"

	// Warmup queue (low priority - separate from campaigns)
	QueueWarmup = "smtpenviador:queue:warmup"

	// Legacy queues (kept for backwards compatibility)
	QueueEmails   = "smtpenviador:queue:emails"
	QueuePriority = "smtpenviador:queue:priority"
	QueueFailed   = "smtpenviador:queue:failed"

	StatsKey     = "smtpenviador:stats"
	RateLimitKey = "smtpenviador:ratelimit"
)

type EmailJob struct {
	ID             string            `json:"id"`
	CampaignID     string            `json:"campaign_id"`
	EmailID        string            `json:"email_id"`
	SMTPID         string            `json:"smtp_id"`
	To             string            `json:"to"`
	ToName         string            `json:"to_name"`
	From           string            `json:"from"`
	FromName       string            `json:"from_name"`
	ReplyTo        string            `json:"reply_to"`
	Subject        string            `json:"subject"`
	HTMLContent    string            `json:"html_content"`
	TextContent    string            `json:"text_content"`
	Variables      map[string]string `json:"variables"`
	TrackOpens     bool              `json:"track_opens"`
	TrackClicks    bool              `json:"track_clicks"`
	TrackingDomain string            `json:"tracking_domain"`
	Retries        int               `json:"retries"`
	CreatedAt      time.Time         `json:"created_at"`
}

type Manager struct {
	client *redis.Client
	ctx    context.Context
}

func Connect(cfg *config.Config) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", cfg.RedisHost, cfg.RedisPort),
		Password: cfg.RedisPassword,
		DB:       0,
	})

	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return client, nil
}

func NewManager(client *redis.Client) *Manager {
	return &Manager{
		client: client,
		ctx:    context.Background(),
	}
}

// Push adds an email job to the queue
func (m *Manager) Push(job *EmailJob) error {
	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	return m.client.LPush(m.ctx, QueueEmails, data).Err()
}

// PushPriority adds an email job to the priority queue
func (m *Manager) PushPriority(job *EmailJob) error {
	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	return m.client.LPush(m.ctx, QueuePriority, data).Err()
}

// Pop gets the next email job from the queue (priority first)
func (m *Manager) Pop() (*EmailJob, error) {
	// Try priority queue first
	data, err := m.client.RPop(m.ctx, QueuePriority).Bytes()
	if err == redis.Nil {
		// Try normal queue
		data, err = m.client.RPop(m.ctx, QueueEmails).Bytes()
		if err == redis.Nil {
			return nil, nil // No jobs available
		}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to pop job: %w", err)
	}

	var job EmailJob
	if err := json.Unmarshal(data, &job); err != nil {
		return nil, fmt.Errorf("failed to unmarshal job: %w", err)
	}

	return &job, nil
}

// PushFailed adds a failed job to the failed queue
func (m *Manager) PushFailed(job *EmailJob, errMsg string) error {
	job.Retries++
	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	return m.client.LPush(m.ctx, QueueFailed, data).Err()
}

// GetQueueLength returns the length of the email queue
func (m *Manager) GetQueueLength() (int64, error) {
	normal, err := m.client.LLen(m.ctx, QueueEmails).Result()
	if err != nil {
		return 0, err
	}
	priority, err := m.client.LLen(m.ctx, QueuePriority).Result()
	if err != nil {
		return 0, err
	}
	return normal + priority, nil
}

// IncrementStat increments a stat counter
func (m *Manager) IncrementStat(stat string, value int64) error {
	key := fmt.Sprintf("%s:%s:%s", StatsKey, time.Now().Format("2006-01-02"), stat)
	return m.client.IncrBy(m.ctx, key, value).Err()
}

// GetStats returns current stats
func (m *Manager) GetStats() (map[string]int64, error) {
	date := time.Now().Format("2006-01-02")
	stats := make(map[string]int64)

	keys := []string{"sent", "failed", "opened", "clicked", "bounced"}
	for _, k := range keys {
		key := fmt.Sprintf("%s:%s:%s", StatsKey, date, k)
		val, _ := m.client.Get(m.ctx, key).Int64()
		stats[k] = val
	}

	return stats, nil
}

// CheckRateLimit checks if we can send from an SMTP (per-minute limit)
func (m *Manager) CheckRateLimit(smtpID string, maxPerMinute int) bool {
	key := fmt.Sprintf("%s:%s:%d", RateLimitKey, smtpID, time.Now().Unix()/60)

	count, err := m.client.Incr(m.ctx, key).Result()
	if err != nil {
		return false
	}

	// Set expiry on first increment
	if count == 1 {
		m.client.Expire(m.ctx, key, 2*time.Minute)
	}

	return count <= int64(maxPerMinute)
}

// CheckRateLimitHourly checks if we can send from an SMTP (per-hour limit)
func (m *Manager) CheckRateLimitHourly(smtpID string, maxPerHour int) bool {
	if maxPerHour <= 0 {
		return true // No hourly limit
	}

	key := fmt.Sprintf("%s:%s:hour:%d", RateLimitKey, smtpID, time.Now().Unix()/3600)

	count, err := m.client.Incr(m.ctx, key).Result()
	if err != nil {
		return false
	}

	// Set expiry on first increment
	if count == 1 {
		m.client.Expire(m.ctx, key, 2*time.Hour)
	}

	return count <= int64(maxPerHour)
}

// CheckBothRateLimits checks both per-minute and per-hour limits
func (m *Manager) CheckBothRateLimits(smtpID string, maxPerMinute, maxPerHour int) bool {
	// Check per-minute first (more likely to hit)
	if !m.CheckRateLimit(smtpID, maxPerMinute) {
		return false
	}
	// Check per-hour
	return m.CheckRateLimitHourly(smtpID, maxPerHour)
}

// GetSMTPSentCount returns how many emails sent from an SMTP in current minute
func (m *Manager) GetSMTPSentCount(smtpID string) int64 {
	key := fmt.Sprintf("%s:%s:%d", RateLimitKey, smtpID, time.Now().Unix()/60)
	count, _ := m.client.Get(m.ctx, key).Int64()
	return count
}

// ==================== CAMPAIGN QUEUE (DEDICATED) ====================

// PushCampaign adds an email job to the campaign queue
func (m *Manager) PushCampaign(job *EmailJob) error {
	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}
	return m.client.LPush(m.ctx, QueueCampaign, data).Err()
}

// PushCampaignPriority adds an email job to the priority campaign queue
func (m *Manager) PushCampaignPriority(job *EmailJob) error {
	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}
	return m.client.LPush(m.ctx, QueueCampaignPriority, data).Err()
}

// PopCampaign gets the next job from campaign queue only
// Also checks legacy queue for backwards compatibility
// Uses BRPOP for efficient blocking - waits up to 1 second for a job
func (m *Manager) PopCampaign() (*EmailJob, error) {
	// Use BRPOP to efficiently wait for jobs from multiple queues
	// Priority: CampaignPriority > Campaign > Legacy
	result, err := m.client.BRPop(m.ctx, 1*time.Second, QueueCampaignPriority, QueueCampaign, QueueEmails).Result()
	if err == redis.Nil {
		return nil, nil // No jobs available after timeout
	}
	if err != nil {
		return nil, fmt.Errorf("failed to pop campaign job: %w", err)
	}

	// BRPop returns [queue_name, value]
	if len(result) < 2 {
		return nil, nil
	}

	var job EmailJob
	if err := json.Unmarshal([]byte(result[1]), &job); err != nil {
		return nil, fmt.Errorf("failed to unmarshal job: %w", err)
	}
	return &job, nil
}

// GetCampaignQueueLength returns the length of the campaign queue
func (m *Manager) GetCampaignQueueLength() (int64, error) {
	normal, err := m.client.LLen(m.ctx, QueueCampaign).Result()
	if err != nil {
		return 0, err
	}
	priority, err := m.client.LLen(m.ctx, QueueCampaignPriority).Result()
	if err != nil {
		return 0, err
	}
	return normal + priority, nil
}

// ==================== WARMUP QUEUE (DEDICATED) ====================

// PushWarmup adds an email job to the warmup queue
func (m *Manager) PushWarmup(job *EmailJob) error {
	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}
	return m.client.LPush(m.ctx, QueueWarmup, data).Err()
}

// PopWarmup gets the next job from warmup queue only
// Uses BRPOP for efficient blocking - waits up to 2 seconds for a job
func (m *Manager) PopWarmup() (*EmailJob, error) {
	result, err := m.client.BRPop(m.ctx, 2*time.Second, QueueWarmup).Result()
	if err == redis.Nil {
		return nil, nil // No jobs available after timeout
	}
	if err != nil {
		return nil, fmt.Errorf("failed to pop warmup job: %w", err)
	}

	// BRPop returns [queue_name, value]
	if len(result) < 2 {
		return nil, nil
	}

	var job EmailJob
	if err := json.Unmarshal([]byte(result[1]), &job); err != nil {
		return nil, fmt.Errorf("failed to unmarshal job: %w", err)
	}
	return &job, nil
}

// GetWarmupQueueLength returns the length of the warmup queue
func (m *Manager) GetWarmupQueueLength() (int64, error) {
	return m.client.LLen(m.ctx, QueueWarmup).Result()
}

// ==================== COMBINED STATS ====================

// GetAllQueueLengths returns lengths for all queues
func (m *Manager) GetAllQueueLengths() (campaign int64, warmup int64, legacy int64, err error) {
	campaign, err = m.GetCampaignQueueLength()
	if err != nil {
		return 0, 0, 0, err
	}
	warmup, err = m.GetWarmupQueueLength()
	if err != nil {
		return 0, 0, 0, err
	}
	legacy, err = m.GetQueueLength()
	if err != nil {
		return 0, 0, 0, err
	}
	return campaign, warmup, legacy, nil
}
