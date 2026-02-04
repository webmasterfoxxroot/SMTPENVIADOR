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
	SMTPIDs        []string          `json:"smtp_ids"` // Allowed SMTPs for this job (user isolation)
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
	BatchSize      int               `json:"batch_size"`     // Emails per batch
	BatchInterval  int               `json:"batch_interval"` // Seconds between batches
	Retries        int               `json:"retries"`
	CreatedAt      time.Time         `json:"created_at"`
}

type Manager struct {
	client *redis.Client
	ctx    context.Context
}

func Connect(cfg *config.Config) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%s", cfg.RedisHost, cfg.RedisPort),
		Password:     cfg.RedisPassword,
		DB:           0,
		PoolSize:     100,             // Connection pool size
		MinIdleConns: 10,              // Minimum idle connections
		DialTimeout:  5 * time.Second, // Connection timeout
		ReadTimeout:  3 * time.Second, // Read timeout
		WriteTimeout: 3 * time.Second, // Write timeout
		PoolTimeout:  4 * time.Second, // Pool timeout
		MaxRetries:   3,               // Max retries on failure
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

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
// Returns true if under limit, false if at/over limit
func (m *Manager) CheckRateLimit(smtpID string, maxPerMinute int) bool {
	key := fmt.Sprintf("%s:%s:%d", RateLimitKey, smtpID, time.Now().Unix()/60)
	count, _ := m.client.Get(m.ctx, key).Int64()
	return count < int64(maxPerMinute)
}

// CheckRateLimitHourly checks if we can send from an SMTP (per-hour limit)
func (m *Manager) CheckRateLimitHourly(smtpID string, maxPerHour int) bool {
	if maxPerHour <= 0 {
		return true // No hourly limit
	}
	key := fmt.Sprintf("%s:%s:hour:%d", RateLimitKey, smtpID, time.Now().Unix()/3600)
	count, _ := m.client.Get(m.ctx, key).Int64()
	return count < int64(maxPerHour)
}

// CheckBothRateLimits checks both per-minute and per-hour limits (read-only)
func (m *Manager) CheckBothRateLimits(smtpID string, maxPerMinute, maxPerHour int) bool {
	if !m.CheckRateLimit(smtpID, maxPerMinute) {
		return false
	}
	return m.CheckRateLimitHourly(smtpID, maxPerHour)
}

// IncrementRateLimit increments the rate limit counters after successful send (legacy/shared)
func (m *Manager) IncrementRateLimit(smtpID string) {
	m.IncrementRateLimitForType(smtpID, "shared")
}

// CheckRateLimitForType checks rate limit for a specific queue type (campaign/warmup)
// Each type has its own separate rate limit pool - campaigns don't compete with warmup
func (m *Manager) CheckRateLimitForType(smtpID string, maxPerMinute int, queueType string) bool {
	key := fmt.Sprintf("%s:%s:%s:%d", RateLimitKey, queueType, smtpID, time.Now().Unix()/60)
	count, _ := m.client.Get(m.ctx, key).Int64()
	return count < int64(maxPerMinute)
}

// CheckRateLimitHourlyForType checks hourly rate limit for a specific queue type
func (m *Manager) CheckRateLimitHourlyForType(smtpID string, maxPerHour int, queueType string) bool {
	if maxPerHour <= 0 {
		return true
	}
	key := fmt.Sprintf("%s:%s:%s:hour:%d", RateLimitKey, queueType, smtpID, time.Now().Unix()/3600)
	count, _ := m.client.Get(m.ctx, key).Int64()
	return count < int64(maxPerHour)
}

// CheckBothRateLimitsForType checks both limits for a specific queue type
func (m *Manager) CheckBothRateLimitsForType(smtpID string, maxPerMinute, maxPerHour int, queueType string) bool {
	if !m.CheckRateLimitForType(smtpID, maxPerMinute, queueType) {
		return false
	}
	return m.CheckRateLimitHourlyForType(smtpID, maxPerHour, queueType)
}

// IncrementRateLimitForType increments rate limit for a specific queue type
func (m *Manager) IncrementRateLimitForType(smtpID string, queueType string) {
	// Increment per-minute counter for this type
	keyMin := fmt.Sprintf("%s:%s:%s:%d", RateLimitKey, queueType, smtpID, time.Now().Unix()/60)
	count, _ := m.client.Incr(m.ctx, keyMin).Result()
	if count == 1 {
		m.client.Expire(m.ctx, keyMin, 2*time.Minute)
	}

	// Increment per-hour counter for this type
	keyHour := fmt.Sprintf("%s:%s:%s:hour:%d", RateLimitKey, queueType, smtpID, time.Now().Unix()/3600)
	count, _ = m.client.Incr(m.ctx, keyHour).Result()
	if count == 1 {
		m.client.Expire(m.ctx, keyHour, 2*time.Hour)
	}
}

// CheckCampaignBatchLimit checks if a campaign can send based on batch_size and batch_interval
// Returns true if can send, false if should wait
// DEPRECATED: Use TryAcquireCampaignBatchSlot for atomic operation to prevent race conditions
func (m *Manager) CheckCampaignBatchLimit(campaignID string, batchSize, batchInterval int) bool {
	if batchInterval <= 0 {
		return true // No interval = unlimited
	}

	// Key format: batch:campaignID:intervalNumber
	intervalNum := time.Now().Unix() / int64(batchInterval)
	key := fmt.Sprintf("batch:%s:%d", campaignID, intervalNum)
	count, _ := m.client.Get(m.ctx, key).Int64()

	return count < int64(batchSize)
}

// TryAcquireCampaignBatchSlot atomically tries to acquire a slot in the batch
// Returns true if slot acquired, false if batch is full for this interval
// This is ATOMIC and safe for concurrent workers (no race condition)
func (m *Manager) TryAcquireCampaignBatchSlot(campaignID string, batchSize, batchInterval int) bool {
	if batchInterval <= 0 {
		return true // No interval = unlimited
	}

	// Key format: batch:campaignID:intervalNumber
	intervalNum := time.Now().Unix() / int64(batchInterval)
	key := fmt.Sprintf("batch:%s:%d", campaignID, intervalNum)

	// Lua script for atomic check-and-increment
	// This script:
	// 1. Gets the current count
	// 2. If count < batchSize, increments and returns 1 (success)
	// 3. If count >= batchSize, returns 0 (batch full)
	// All operations are atomic within Redis
	luaScript := `
		local key = KEYS[1]
		local batchSize = tonumber(ARGV[1])
		local ttl = tonumber(ARGV[2])

		local current = tonumber(redis.call('GET', key) or '0')

		if current < batchSize then
			local newCount = redis.call('INCR', key)
			if newCount == 1 then
				redis.call('EXPIRE', key, ttl)
			end
			return 1
		end

		return 0
	`

	ttl := batchInterval * 2 // Expiry = 2x interval
	result, err := m.client.Eval(m.ctx, luaScript, []string{key}, batchSize, ttl).Int64()
	if err != nil {
		// On error, fall back to non-atomic check (better than blocking everything)
		return m.CheckCampaignBatchLimit(campaignID, batchSize, batchInterval)
	}

	return result == 1
}

// IncrementCampaignBatchCount increments the batch counter for a campaign
// DEPRECATED: Use TryAcquireCampaignBatchSlot which does increment atomically
func (m *Manager) IncrementCampaignBatchCount(campaignID string, batchInterval int) {
	if batchInterval <= 0 {
		return // No tracking needed
	}

	intervalNum := time.Now().Unix() / int64(batchInterval)
	key := fmt.Sprintf("batch:%s:%d", campaignID, intervalNum)
	count, _ := m.client.Incr(m.ctx, key).Result()
	if count == 1 {
		// Set expiry to 2x interval to ensure cleanup
		m.client.Expire(m.ctx, key, time.Duration(batchInterval*2)*time.Second)
	}
}

// GetSMTPSentCount returns how many emails sent from an SMTP in current minute
func (m *Manager) GetSMTPSentCount(smtpID string) int64 {
	key := fmt.Sprintf("%s:%s:%d", RateLimitKey, smtpID, time.Now().Unix()/60)
	count, _ := m.client.Get(m.ctx, key).Int64()
	return count
}

// GetSMTPRemainingCapacity returns remaining capacity for an SMTP in current minute for a queue type
func (m *Manager) GetSMTPRemainingCapacity(smtpID string, maxPerMinute int, queueType string) int64 {
	key := fmt.Sprintf("%s:%s:%s:%d", RateLimitKey, queueType, smtpID, time.Now().Unix()/60)
	count, _ := m.client.Get(m.ctx, key).Int64()
	remaining := int64(maxPerMinute) - count
	if remaining < 0 {
		return 0
	}
	return remaining
}

// ==================== CAMPAIGN QUEUE (DEDICATED) ====================

// PushCampaign adds an email job to the campaign-specific queue
// Each campaign has its own queue for fair processing
func (m *Manager) PushCampaign(job *EmailJob) error {
	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	// Use campaign-specific queue for fair distribution
	if job.CampaignID != "" {
		campaignQueue := fmt.Sprintf("%s:%s", QueueCampaign, job.CampaignID)
		// Register this campaign as active (with short TTL to auto-cleanup)
		m.client.SAdd(m.ctx, "smtpenviador:active_campaigns", job.CampaignID)
		m.client.Expire(m.ctx, "smtpenviador:active_campaigns", 1*time.Hour)
		return m.client.LPush(m.ctx, campaignQueue, data).Err()
	}

	// Fallback to main queue if no campaign ID
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

// PushCampaignBatch adds multiple email jobs to the campaign queue using pipeline
// This is much more efficient than pushing one at a time for large batches
func (m *Manager) PushCampaignBatch(jobs []*EmailJob) error {
	if len(jobs) == 0 {
		return nil
	}

	// Group jobs by campaign ID for efficient queueing
	campaignJobs := make(map[string][]interface{})
	for _, job := range jobs {
		data, err := json.Marshal(job)
		if err != nil {
			continue
		}
		queueKey := QueueCampaign
		if job.CampaignID != "" {
			queueKey = fmt.Sprintf("%s:%s", QueueCampaign, job.CampaignID)
		}
		campaignJobs[queueKey] = append(campaignJobs[queueKey], data)
	}

	// Use pipeline for batch operations (much faster)
	pipe := m.client.Pipeline()
	campaignIDs := make([]string, 0)

	for queueKey, data := range campaignJobs {
		pipe.LPush(m.ctx, queueKey, data...)
		// Extract campaign ID from queue key
		if len(queueKey) > len(QueueCampaign)+1 {
			campaignID := queueKey[len(QueueCampaign)+1:]
			campaignIDs = append(campaignIDs, campaignID)
		}
	}

	// Register all campaigns as active
	if len(campaignIDs) > 0 {
		campaignIDsInterface := make([]interface{}, len(campaignIDs))
		for i, id := range campaignIDs {
			campaignIDsInterface[i] = id
		}
		pipe.SAdd(m.ctx, "smtpenviador:active_campaigns", campaignIDsInterface...)
		pipe.Expire(m.ctx, "smtpenviador:active_campaigns", 1*time.Hour)
	}

	_, err := pipe.Exec(m.ctx)
	return err
}

// PopCampaign gets the next job using round-robin across all active campaigns
// Uses efficient BRPOP to avoid busy-loop when no jobs available
func (m *Manager) PopCampaign() (*EmailJob, error) {
	// Get all active campaign queues
	campaigns, err := m.client.SMembers(m.ctx, "smtpenviador:active_campaigns").Result()
	if err != nil || len(campaigns) == 0 {
		// No active campaigns, use blocking pop on legacy queues
		result, err := m.client.BRPop(m.ctx, 1*time.Second, QueueCampaignPriority, QueueCampaign, QueueEmails).Result()
		if err == redis.Nil {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("failed to pop job: %w", err)
		}
		if len(result) < 2 {
			return nil, nil
		}
		var job EmailJob
		if err := json.Unmarshal([]byte(result[1]), &job); err != nil {
			return nil, fmt.Errorf("failed to unmarshal job: %w", err)
		}
		return &job, nil
	}

	// Build list of all queues to check (priority + campaign-specific + legacy)
	queues := make([]string, 0, len(campaigns)+3)
	queues = append(queues, QueueCampaignPriority) // Priority first

	// Add campaign-specific queues in round-robin order
	idx, _ := m.client.Incr(m.ctx, "smtpenviador:campaign_rr_index").Result()
	for i := 0; i < len(campaigns); i++ {
		campaignIdx := (int(idx) + i) % len(campaigns)
		campaignQueue := fmt.Sprintf("%s:%s", QueueCampaign, campaigns[campaignIdx])
		queues = append(queues, campaignQueue)
	}

	// Add legacy queues as fallback
	queues = append(queues, QueueCampaign, QueueEmails)

	// Use BRPOP on all queues - efficient blocking wait
	result, err := m.client.BRPop(m.ctx, 1*time.Second, queues...).Result()
	if err == redis.Nil {
		return nil, nil // No jobs available
	}
	if err != nil {
		return nil, fmt.Errorf("failed to pop job: %w", err)
	}
	if len(result) < 2 {
		return nil, nil
	}

	var job EmailJob
	if err := json.Unmarshal([]byte(result[1]), &job); err != nil {
		return nil, fmt.Errorf("failed to unmarshal job: %w", err)
	}
	return &job, nil
}

// GetCampaignQueueLength returns the total length of all campaign queues
func (m *Manager) GetCampaignQueueLength() (int64, error) {
	var total int64

	// Count main queue
	normal, err := m.client.LLen(m.ctx, QueueCampaign).Result()
	if err == nil {
		total += normal
	}

	// Count priority queue
	priority, err := m.client.LLen(m.ctx, QueueCampaignPriority).Result()
	if err == nil {
		total += priority
	}

	// Count all campaign-specific queues
	campaigns, err := m.client.SMembers(m.ctx, "smtpenviador:active_campaigns").Result()
	if err == nil {
		for _, campaignID := range campaigns {
			campaignQueue := fmt.Sprintf("%s:%s", QueueCampaign, campaignID)
			qLen, err := m.client.LLen(m.ctx, campaignQueue).Result()
			if err == nil {
				total += qLen
			}
		}
	}

	return total, nil
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

// GetQueueLengthByKey returns the length of a specific queue by key
func (m *Manager) GetQueueLengthByKey(key string) (int64, error) {
	return m.client.LLen(m.ctx, key).Result()
}

// ==================== REQUEUE LOCK (PREVENT DUPLICATE REQUEUE) ====================

// IsRequeueInProgress checks if a requeue operation is already in progress
func (m *Manager) IsRequeueInProgress(lockKey string) bool {
	exists, _ := m.client.Exists(m.ctx, lockKey).Result()
	return exists > 0
}

// SetRequeueLock sets a lock to prevent duplicate requeue operations
func (m *Manager) SetRequeueLock(lockKey string) error {
	// Lock expires in 5 minutes (max time for requeue operation)
	return m.client.Set(m.ctx, lockKey, "1", 5*time.Minute).Err()
}

// ClearRequeueLock removes the requeue lock
func (m *Manager) ClearRequeueLock(lockKey string) error {
	return m.client.Del(m.ctx, lockKey).Err()
}

// ==================== CAMPAIGN STATUS CACHE ====================

const (
	// CampaignStatusKey is the prefix for campaign status cache in Redis
	CampaignStatusKey = "smtpenviador:campaign:status"
	// Campaign status cache TTL (1 hour)
	CampaignStatusTTL = 1 * time.Hour
)

// SetCampaignStatus sets the campaign status in Redis cache
// This should be called when campaign status changes (pause, resume, etc.)
func (m *Manager) SetCampaignStatus(campaignID, status string) error {
	key := fmt.Sprintf("%s:%s", CampaignStatusKey, campaignID)
	return m.client.Set(m.ctx, key, status, CampaignStatusTTL).Err()
}

// GetCampaignStatus gets the campaign status from Redis cache
// Returns empty string if not cached
func (m *Manager) GetCampaignStatus(campaignID string) string {
	key := fmt.Sprintf("%s:%s", CampaignStatusKey, campaignID)
	status, err := m.client.Get(m.ctx, key).Result()
	if err == redis.Nil || err != nil {
		return "" // Not cached
	}
	return status
}

// IsCampaignPaused checks if a campaign is paused (using Redis cache)
// Returns true if campaign is paused, false otherwise
func (m *Manager) IsCampaignPaused(campaignID string) bool {
	status := m.GetCampaignStatus(campaignID)
	return status == "paused"
}

// ClearCampaignStatus removes the campaign status from cache
func (m *Manager) ClearCampaignStatus(campaignID string) error {
	key := fmt.Sprintf("%s:%s", CampaignStatusKey, campaignID)
	return m.client.Del(m.ctx, key).Err()
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
