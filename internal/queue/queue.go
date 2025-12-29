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
	QueueEmails    = "smtpenviador:queue:emails"
	QueuePriority  = "smtpenviador:queue:priority"
	QueueFailed    = "smtpenviador:queue:failed"
	StatsKey       = "smtpenviador:stats"
	RateLimitKey   = "smtpenviador:ratelimit"
)

type EmailJob struct {
	ID          string            `json:"id"`
	CampaignID  string            `json:"campaign_id"`
	EmailID     string            `json:"email_id"`
	SMTPID      string            `json:"smtp_id"`
	To          string            `json:"to"`
	ToName      string            `json:"to_name"`
	From        string            `json:"from"`
	FromName    string            `json:"from_name"`
	ReplyTo     string            `json:"reply_to"`
	Subject     string            `json:"subject"`
	HTMLContent string            `json:"html_content"`
	TextContent string            `json:"text_content"`
	Variables   map[string]string `json:"variables"`
	Retries     int               `json:"retries"`
	CreatedAt   time.Time         `json:"created_at"`
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

// CheckRateLimit checks if we can send from an SMTP
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

// GetSMTPSentCount returns how many emails sent from an SMTP in current minute
func (m *Manager) GetSMTPSentCount(smtpID string) int64 {
	key := fmt.Sprintf("%s:%s:%d", RateLimitKey, smtpID, time.Now().Unix()/60)
	count, _ := m.client.Get(m.ctx, key).Int64()
	return count
}
