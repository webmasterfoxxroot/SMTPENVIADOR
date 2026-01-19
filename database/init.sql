-- SMTPENVIADOR Database Schema

-- Extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Users table
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    role VARCHAR(50) DEFAULT 'user', -- admin, user
    active BOOLEAN DEFAULT true,
    tracking_domain VARCHAR(500), -- User's tracking domain for email tracking
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- SMTP Servers table
CREATE TABLE smtp_servers (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    host VARCHAR(255) NOT NULL,
    port INTEGER NOT NULL DEFAULT 587,
    username VARCHAR(255) NOT NULL,
    password VARCHAR(255) NOT NULL,
    tls_mode VARCHAR(20) DEFAULT 'starttls', -- none, starttls, tls (implicit)
    max_per_minute INTEGER DEFAULT 1000,
    max_per_hour INTEGER DEFAULT 50000,
    max_connections INTEGER DEFAULT 5,
    active BOOLEAN DEFAULT true,
    status VARCHAR(50) DEFAULT 'unknown', -- online, offline, error, unknown
    last_check TIMESTAMP,
    total_sent BIGINT DEFAULT 0,
    total_failed BIGINT DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_smtp_servers_user_id ON smtp_servers(user_id);

-- SMTP Senders table (multiple senders per SMTP)
CREATE TABLE smtp_senders (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    smtp_id UUID REFERENCES smtp_servers(id) ON DELETE CASCADE,
    email VARCHAR(255) NOT NULL,
    name VARCHAR(255),
    reply_to VARCHAR(255),
    active BOOLEAN DEFAULT true,
    total_sent BIGINT DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(smtp_id, email)
);

CREATE INDEX idx_smtp_senders_smtp_id ON smtp_senders(smtp_id);

-- Email List Groups table
CREATE TABLE email_list_groups (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    color VARCHAR(20) DEFAULT '#3B82F6',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_email_list_groups_user_id ON email_list_groups(user_id);

-- Email Lists table
CREATE TABLE email_lists (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    total_emails INTEGER DEFAULT 0,
    valid_emails INTEGER DEFAULT 0,
    invalid_emails INTEGER DEFAULT 0,
    status VARCHAR(50) DEFAULT 'pending', -- pending, processing, ready, error
    group_id UUID REFERENCES email_list_groups(id) ON DELETE SET NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_email_lists_user_id ON email_lists(user_id);

-- Emails table (subscribers)
CREATE TABLE emails (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    list_id UUID REFERENCES email_lists(id) ON DELETE CASCADE,
    email VARCHAR(255) NOT NULL,
    name VARCHAR(255),
    custom1 VARCHAR(255),
    custom2 VARCHAR(255),
    custom3 VARCHAR(255),
    custom4 VARCHAR(255),
    custom5 VARCHAR(255),
    custom6 VARCHAR(255),
    custom7 VARCHAR(255),
    custom8 VARCHAR(255),
    custom9 VARCHAR(255),
    custom10 VARCHAR(255),
    valid BOOLEAN DEFAULT true,
    bounced BOOLEAN DEFAULT false,
    unsubscribed BOOLEAN DEFAULT false,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(list_id, email)
);

-- Create index for faster lookups
CREATE INDEX idx_emails_list_id ON emails(list_id);
CREATE INDEX idx_emails_email ON emails(email);
CREATE INDEX idx_emails_valid ON emails(valid);

-- Templates table
CREATE TABLE templates (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    from_name VARCHAR(255),
    subject VARCHAR(500) NOT NULL,
    html_content TEXT NOT NULL,
    text_content TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_templates_user_id ON templates(user_id);

-- Campaigns table
CREATE TABLE campaigns (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    subject VARCHAR(500) NOT NULL,
    from_name VARCHAR(255) NOT NULL,
    from_email VARCHAR(255) NOT NULL,
    reply_to VARCHAR(255),
    html_content TEXT NOT NULL,
    text_content TEXT,
    list_id UUID REFERENCES email_lists(id),
    list_ids TEXT DEFAULT '', -- Comma-separated list IDs for multiple list support
    smtp_ids TEXT DEFAULT '', -- Comma-separated SMTP IDs for multiple SMTP support
    status VARCHAR(50) DEFAULT 'draft', -- draft, scheduled, running, paused, completed, cancelled
    scheduled_at TIMESTAMP,
    auto_start_at TIMESTAMP, -- when campaign should auto-start (countdown)
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    total_emails INTEGER DEFAULT 0,
    sent_count INTEGER DEFAULT 0,
    failed_count INTEGER DEFAULT 0,
    open_count INTEGER DEFAULT 0,
    click_count INTEGER DEFAULT 0,
    bounce_count INTEGER DEFAULT 0,
    unsubscribe_count INTEGER DEFAULT 0,
    send_rate INTEGER DEFAULT 0, -- emails per minute, 0 = unlimited
    track_opens BOOLEAN DEFAULT true, -- enable/disable open tracking
    track_clicks BOOLEAN DEFAULT true, -- enable/disable click tracking
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_campaigns_user_id ON campaigns(user_id);

-- Campaign Emails (queue)
-- Note: email_id references ClickHouse emails, not PostgreSQL
CREATE TABLE campaign_emails (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    campaign_id UUID REFERENCES campaigns(id) ON DELETE CASCADE,
    email_id UUID NOT NULL, -- References emails in ClickHouse, no FK constraint
    smtp_id UUID REFERENCES smtp_servers(id),
    status VARCHAR(50) DEFAULT 'pending', -- pending, queued, sending, sent, failed, bounced
    error_message TEXT,
    sent_at TIMESTAMP,
    opened_at TIMESTAMP,
    clicked_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_campaign_emails_campaign_id ON campaign_emails(campaign_id);
CREATE INDEX idx_campaign_emails_status ON campaign_emails(status);

-- Tracking Events table
-- Note: email_id references ClickHouse emails, not PostgreSQL
CREATE TABLE tracking_events (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    campaign_id UUID REFERENCES campaigns(id) ON DELETE CASCADE,
    email_id UUID NOT NULL, -- References emails in ClickHouse, no FK constraint
    event_type VARCHAR(50) NOT NULL, -- open, click, bounce, unsubscribe
    link_url TEXT,
    ip_address VARCHAR(45),
    user_agent TEXT,
    -- Geolocation fields
    country VARCHAR(100),
    country_code VARCHAR(10), -- ISO 3166-1 alpha-2 code (e.g., BR, US)
    region VARCHAR(100),
    city VARCHAR(100),
    lat DECIMAL(10, 8),
    lon DECIMAL(11, 8),
    timezone VARCHAR(100),
    isp VARCHAR(255),
    -- Device/browser fields
    browser VARCHAR(100),
    browser_version VARCHAR(50),
    os VARCHAR(100),
    os_version VARCHAR(50),
    device VARCHAR(100),
    device_type VARCHAR(50), -- desktop, mobile, tablet, bot
    email_client VARCHAR(100),
    -- Bot detection fields
    is_bot BOOLEAN DEFAULT false,
    is_suspicious BOOLEAN DEFAULT false,
    bot_type VARCHAR(100), -- search_engine, social_media, security_scanner, etc
    bot_name VARCHAR(100), -- Googlebot, Barracuda, etc
    bot_score INTEGER DEFAULT 0, -- 0-100 (higher = more likely bot)
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_tracking_events_campaign_id ON tracking_events(campaign_id);
CREATE INDEX idx_tracking_events_event_type ON tracking_events(event_type);
CREATE INDEX idx_tracking_events_country ON tracking_events(country);
CREATE INDEX idx_tracking_events_country_code ON tracking_events(country_code);
CREATE INDEX idx_tracking_events_city ON tracking_events(city);
CREATE INDEX idx_tracking_events_device_type ON tracking_events(device_type);
CREATE INDEX idx_tracking_events_browser ON tracking_events(browser);
CREATE INDEX idx_tracking_events_is_bot ON tracking_events(is_bot);
CREATE INDEX idx_tracking_events_created_at ON tracking_events(created_at);
CREATE INDEX idx_tracking_events_campaign_bot ON tracking_events(campaign_id, is_bot);
CREATE INDEX idx_tracking_events_campaign_country ON tracking_events(campaign_id, country);

-- Blacklist table
CREATE TABLE blacklist (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    email VARCHAR(255) NOT NULL,
    reason VARCHAR(255), -- bounce, unsubscribe, complaint, manual
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, email)
);

CREATE INDEX idx_blacklist_user_email ON blacklist(user_id, email);

-- Stats table (for real-time dashboard)
CREATE TABLE stats (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    date DATE NOT NULL DEFAULT CURRENT_DATE,
    hour INTEGER NOT NULL DEFAULT 0,
    sent_count INTEGER DEFAULT 0,
    failed_count INTEGER DEFAULT 0,
    open_count INTEGER DEFAULT 0,
    click_count INTEGER DEFAULT 0,
    bounce_count INTEGER DEFAULT 0,
    UNIQUE(date, hour)
);

-- Logs table
CREATE TABLE logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    level VARCHAR(20) NOT NULL, -- info, warning, error
    message TEXT NOT NULL,
    details JSONB,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_logs_level ON logs(level);
CREATE INDEX idx_logs_created_at ON logs(created_at);

-- Settings table
CREATE TABLE settings (
    key VARCHAR(100) PRIMARY KEY,
    value TEXT NOT NULL,
    description VARCHAR(255),
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Import Jobs table (persistent job tracking)
CREATE TABLE import_jobs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    list_id UUID REFERENCES email_lists(id) ON DELETE CASCADE,
    file_name VARCHAR(500) NOT NULL,
    file_path VARCHAR(500),
    status VARCHAR(50) DEFAULT 'pending', -- pending, validating, importing, completed, failed
    total_lines INTEGER DEFAULT 0,
    processed INTEGER DEFAULT 0,
    valid INTEGER DEFAULT 0,
    invalid INTEGER DEFAULT 0,
    duplicates INTEGER DEFAULT 0,
    error_message TEXT,
    has_header BOOLEAN DEFAULT false,
    delimiter VARCHAR(10) DEFAULT ',',
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_import_jobs_list_id ON import_jobs(list_id);
CREATE INDEX idx_import_jobs_status ON import_jobs(status);

-- Insert default settings
INSERT INTO settings (key, value, description) VALUES
('tracking_domain', 'http://localhost', 'Domain for tracking opens and clicks'),
('workers_count', '10', 'Number of email sending workers'),
('connections_per_smtp', '5', 'Max connections per SMTP server'),
('default_send_rate', '0', 'Default send rate (emails/min, 0=unlimited)'),
('retry_attempts', '3', 'Number of retry attempts for failed emails'),
('retry_delay', '60', 'Delay between retries (seconds)'),
('max_upload_size_mb', '500', 'Maximum file upload size in MB'),
('import_batch_size', '1000', 'Quantidade de emails processados por vez durante importação');

-- Insert default admin user (password: admin123)
INSERT INTO users (email, password_hash, name, role) VALUES
('admin@admin.com', '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy', 'Administrador', 'admin');

-- Create updated_at trigger function
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Apply trigger to tables
CREATE TRIGGER update_users_updated_at BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_smtp_servers_updated_at BEFORE UPDATE ON smtp_servers FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_email_list_groups_updated_at BEFORE UPDATE ON email_list_groups FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_email_lists_updated_at BEFORE UPDATE ON email_lists FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_templates_updated_at BEFORE UPDATE ON templates FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_campaigns_updated_at BEFORE UPDATE ON campaigns FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
