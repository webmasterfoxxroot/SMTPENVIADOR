-- Migration: Add performance indexes for campaigns
-- This migration adds critical indexes to improve campaign performance

-- ============================================
-- EMAILS TABLE INDEXES
-- ============================================

-- Composite index for email filtering during campaign start
-- This is CRITICAL for large lists - speeds up the LEFT JOIN on blacklist
CREATE INDEX IF NOT EXISTS idx_emails_list_valid_status
ON emails(list_id, valid, bounced, unsubscribed)
WHERE valid = true AND bounced = false AND unsubscribed = false;

-- Index for email lookups (used in tracking)
CREATE INDEX IF NOT EXISTS idx_emails_id_email ON emails(id, email);

-- ============================================
-- BLACKLIST TABLE INDEXES
-- ============================================

-- Index for fast email lookups with LOWER() function
-- Uses expression index for case-insensitive matching
CREATE INDEX IF NOT EXISTS idx_blacklist_email_lower ON blacklist(LOWER(email));

-- ============================================
-- CAMPAIGN_EMAILS TABLE INDEXES
-- ============================================

-- Composite index for campaign + status queries
CREATE INDEX IF NOT EXISTS idx_campaign_emails_campaign_status
ON campaign_emails(campaign_id, status);

-- Index for sent_at queries (for timeline/reports)
CREATE INDEX IF NOT EXISTS idx_campaign_emails_sent_at
ON campaign_emails(campaign_id, sent_at)
WHERE sent_at IS NOT NULL;

-- Index for opened_at queries (for open tracking reports)
CREATE INDEX IF NOT EXISTS idx_campaign_emails_opened_at
ON campaign_emails(campaign_id, opened_at)
WHERE opened_at IS NOT NULL;

-- Index for clicked_at queries (for click tracking reports)
CREATE INDEX IF NOT EXISTS idx_campaign_emails_clicked_at
ON campaign_emails(campaign_id, clicked_at)
WHERE clicked_at IS NOT NULL;

-- Index for email_id lookups (used in tracking)
CREATE INDEX IF NOT EXISTS idx_campaign_emails_email_id
ON campaign_emails(email_id);

-- ============================================
-- TRACKING_EVENTS TABLE INDEXES
-- ============================================

-- Composite index for campaign + timestamp (for time-series queries)
CREATE INDEX IF NOT EXISTS idx_tracking_events_campaign_created
ON tracking_events(campaign_id, created_at DESC);

-- Index for event_type + campaign (for stats aggregation)
CREATE INDEX IF NOT EXISTS idx_tracking_events_type_campaign
ON tracking_events(event_type, campaign_id);

-- Index for bot filtering in stats queries
CREATE INDEX IF NOT EXISTS idx_tracking_events_bot_filter
ON tracking_events(campaign_id, is_bot, event_type)
WHERE is_bot = false OR is_bot IS NULL;

-- ============================================
-- CAMPAIGNS TABLE INDEXES
-- ============================================

-- Index for status queries (draft, running, completed, etc.)
CREATE INDEX IF NOT EXISTS idx_campaigns_status
ON campaigns(status);

-- Composite index for user + status (common dashboard query)
CREATE INDEX IF NOT EXISTS idx_campaigns_user_status
ON campaigns(user_id, status);

-- Index for auto_start_at (for auto-start feature)
CREATE INDEX IF NOT EXISTS idx_campaigns_auto_start
ON campaigns(auto_start_at)
WHERE status = 'draft' AND auto_start_at IS NOT NULL;

-- ============================================
-- STATS TABLE INDEXES
-- ============================================

-- Index for date range queries
CREATE INDEX IF NOT EXISTS idx_stats_date_hour
ON stats(date DESC, hour);

-- ============================================
-- ANALYZE TABLES
-- ============================================
-- Update statistics for query planner
ANALYZE emails;
ANALYZE blacklist;
ANALYZE campaign_emails;
ANALYZE tracking_events;
ANALYZE campaigns;
ANALYZE stats;
