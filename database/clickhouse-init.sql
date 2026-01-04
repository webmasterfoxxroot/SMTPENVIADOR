-- ClickHouse Database Initialization
-- Optimized for billions of records with high compression

-- Create database
CREATE DATABASE IF NOT EXISTS smtpenviador;

-- ===========================================
-- EMAILS TABLE - Optimized for bulk storage
-- ===========================================
CREATE TABLE IF NOT EXISTS smtpenviador.emails
(
    id UUID DEFAULT generateUUIDv4(),
    list_id UUID NOT NULL,
    email String NOT NULL,
    email_hash UInt64 MATERIALIZED cityHash64(lower(email)),
    name String DEFAULT '',
    custom1 String DEFAULT '',
    custom2 String DEFAULT '',
    custom3 String DEFAULT '',
    custom4 String DEFAULT '',
    custom5 String DEFAULT '',
    valid UInt8 DEFAULT 1,
    bounced UInt8 DEFAULT 0,
    unsubscribed UInt8 DEFAULT 0,
    created_at DateTime DEFAULT now(),
    updated_at DateTime DEFAULT now()
)
ENGINE = MergeTree()
PARTITION BY toYYYYMM(created_at)
ORDER BY (list_id, email_hash)
SETTINGS index_granularity = 8192;

-- Index for email lookups
CREATE INDEX IF NOT EXISTS idx_email ON smtpenviador.emails (lower(email)) TYPE bloom_filter GRANULARITY 4;

-- ===========================================
-- BLACKLIST TABLE - Fast email blocking
-- ===========================================
CREATE TABLE IF NOT EXISTS smtpenviador.blacklist
(
    id UUID DEFAULT generateUUIDv4(),
    email String NOT NULL,
    email_hash UInt64 MATERIALIZED cityHash64(lower(email)),
    reason String DEFAULT 'import',
    source String DEFAULT '',
    created_at DateTime DEFAULT now()
)
ENGINE = ReplacingMergeTree(created_at)
ORDER BY email_hash
SETTINGS index_granularity = 8192;

-- Bloom filter for fast lookups
CREATE INDEX IF NOT EXISTS idx_blacklist_email ON smtpenviador.blacklist (lower(email)) TYPE bloom_filter GRANULARITY 4;

-- ===========================================
-- SEND LOGS TABLE - Campaign sending history
-- ===========================================
CREATE TABLE IF NOT EXISTS smtpenviador.send_logs
(
    id UUID DEFAULT generateUUIDv4(),
    campaign_id UUID NOT NULL,
    email_id UUID NOT NULL,
    email String NOT NULL,
    smtp_server_id UUID,
    status Enum8('queued' = 0, 'sent' = 1, 'failed' = 2, 'bounced' = 3, 'opened' = 4, 'clicked' = 5),
    error_message String DEFAULT '',
    sent_at DateTime DEFAULT now(),
    opened_at Nullable(DateTime),
    clicked_at Nullable(DateTime)
)
ENGINE = MergeTree()
PARTITION BY toYYYYMMDD(sent_at)
ORDER BY (campaign_id, sent_at)
SETTINGS index_granularity = 8192;

-- ===========================================
-- STATISTICS AGGREGATION (Materialized View)
-- ===========================================
CREATE MATERIALIZED VIEW IF NOT EXISTS smtpenviador.campaign_stats_mv
ENGINE = SummingMergeTree()
PARTITION BY toYYYYMM(date)
ORDER BY (campaign_id, date)
AS SELECT
    campaign_id,
    toDate(sent_at) AS date,
    count() AS total_sent,
    countIf(status = 'sent') AS successful,
    countIf(status = 'failed') AS failed,
    countIf(status = 'bounced') AS bounced,
    countIf(status = 'opened') AS opened,
    countIf(status = 'clicked') AS clicked
FROM smtpenviador.send_logs
GROUP BY campaign_id, date;

-- ===========================================
-- BLACKLIST IMPORT JOBS TABLE
-- ===========================================
CREATE TABLE IF NOT EXISTS smtpenviador.blacklist_import_jobs
(
    id UUID DEFAULT generateUUIDv4(),
    file_name String NOT NULL,
    file_path String DEFAULT '',
    db_type String NOT NULL,
    status Enum8('pending' = 0, 'processing' = 1, 'completed' = 2, 'failed' = 3, 'cancelled' = 4) DEFAULT 'pending',
    total_emails UInt64 DEFAULT 0,
    processed UInt64 DEFAULT 0,
    imported UInt64 DEFAULT 0,
    duplicates UInt64 DEFAULT 0,
    errors UInt64 DEFAULT 0,
    error_message String DEFAULT '',
    started_at Nullable(DateTime),
    completed_at Nullable(DateTime),
    created_at DateTime DEFAULT now(),
    updated_at DateTime DEFAULT now()
)
ENGINE = MergeTree()
ORDER BY created_at
SETTINGS index_granularity = 8192;

-- ===========================================
-- EMAIL IMPORT JOBS TABLE
-- ===========================================
CREATE TABLE IF NOT EXISTS smtpenviador.email_import_jobs
(
    id UUID DEFAULT generateUUIDv4(),
    list_id UUID NOT NULL,
    file_name String NOT NULL,
    file_path String DEFAULT '',
    status Enum8('pending' = 0, 'processing' = 1, 'completed' = 2, 'failed' = 3, 'cancelled' = 4) DEFAULT 'pending',
    total_lines UInt64 DEFAULT 0,
    processed UInt64 DEFAULT 0,
    valid UInt64 DEFAULT 0,
    invalid UInt64 DEFAULT 0,
    duplicates UInt64 DEFAULT 0,
    error_message String DEFAULT '',
    started_at Nullable(DateTime),
    completed_at Nullable(DateTime),
    created_at DateTime DEFAULT now(),
    updated_at DateTime DEFAULT now()
)
ENGINE = MergeTree()
ORDER BY (list_id, created_at)
SETTINGS index_granularity = 8192;
