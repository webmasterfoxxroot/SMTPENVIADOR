-- Migration: Add geolocation and device tracking fields to tracking_events table
-- This migration adds comprehensive tracking capabilities including:
-- - Geolocation (country, city, region, coordinates)
-- - Device detection (browser, OS, device type)
-- - Bot detection (is_bot, bot_type, bot_score)

-- Add geolocation columns
ALTER TABLE tracking_events ADD COLUMN IF NOT EXISTS country_code VARCHAR(10);
ALTER TABLE tracking_events ADD COLUMN IF NOT EXISTS region VARCHAR(100);
ALTER TABLE tracking_events ADD COLUMN IF NOT EXISTS lat DECIMAL(10, 8);
ALTER TABLE tracking_events ADD COLUMN IF NOT EXISTS lon DECIMAL(11, 8);
ALTER TABLE tracking_events ADD COLUMN IF NOT EXISTS timezone VARCHAR(100);
ALTER TABLE tracking_events ADD COLUMN IF NOT EXISTS isp VARCHAR(255);

-- Add device/browser columns
ALTER TABLE tracking_events ADD COLUMN IF NOT EXISTS browser VARCHAR(100);
ALTER TABLE tracking_events ADD COLUMN IF NOT EXISTS browser_version VARCHAR(50);
ALTER TABLE tracking_events ADD COLUMN IF NOT EXISTS os VARCHAR(100);
ALTER TABLE tracking_events ADD COLUMN IF NOT EXISTS os_version VARCHAR(50);
ALTER TABLE tracking_events ADD COLUMN IF NOT EXISTS device VARCHAR(100);
ALTER TABLE tracking_events ADD COLUMN IF NOT EXISTS device_type VARCHAR(50);
ALTER TABLE tracking_events ADD COLUMN IF NOT EXISTS email_client VARCHAR(100);

-- Add bot detection columns
ALTER TABLE tracking_events ADD COLUMN IF NOT EXISTS is_bot BOOLEAN DEFAULT false;
ALTER TABLE tracking_events ADD COLUMN IF NOT EXISTS is_suspicious BOOLEAN DEFAULT false;
ALTER TABLE tracking_events ADD COLUMN IF NOT EXISTS bot_type VARCHAR(100);
ALTER TABLE tracking_events ADD COLUMN IF NOT EXISTS bot_name VARCHAR(100);
ALTER TABLE tracking_events ADD COLUMN IF NOT EXISTS bot_score INTEGER DEFAULT 0;

-- Create indexes for common queries
CREATE INDEX IF NOT EXISTS idx_tracking_events_country ON tracking_events(country);
CREATE INDEX IF NOT EXISTS idx_tracking_events_country_code ON tracking_events(country_code);
CREATE INDEX IF NOT EXISTS idx_tracking_events_city ON tracking_events(city);
CREATE INDEX IF NOT EXISTS idx_tracking_events_device_type ON tracking_events(device_type);
CREATE INDEX IF NOT EXISTS idx_tracking_events_browser ON tracking_events(browser);
CREATE INDEX IF NOT EXISTS idx_tracking_events_is_bot ON tracking_events(is_bot);
CREATE INDEX IF NOT EXISTS idx_tracking_events_created_at ON tracking_events(created_at);

-- Create composite index for dashboard queries
CREATE INDEX IF NOT EXISTS idx_tracking_events_campaign_bot ON tracking_events(campaign_id, is_bot);
CREATE INDEX IF NOT EXISTS idx_tracking_events_campaign_country ON tracking_events(campaign_id, country);

-- Comment on new columns for documentation
COMMENT ON COLUMN tracking_events.country_code IS 'ISO 3166-1 alpha-2 country code (e.g., BR, US)';
COMMENT ON COLUMN tracking_events.region IS 'State/region name';
COMMENT ON COLUMN tracking_events.lat IS 'Latitude coordinate';
COMMENT ON COLUMN tracking_events.lon IS 'Longitude coordinate';
COMMENT ON COLUMN tracking_events.timezone IS 'Timezone (e.g., America/Sao_Paulo)';
COMMENT ON COLUMN tracking_events.isp IS 'Internet Service Provider';
COMMENT ON COLUMN tracking_events.browser IS 'Browser name (e.g., Chrome, Firefox)';
COMMENT ON COLUMN tracking_events.browser_version IS 'Browser version';
COMMENT ON COLUMN tracking_events.os IS 'Operating system (e.g., Windows, macOS, iOS)';
COMMENT ON COLUMN tracking_events.os_version IS 'OS version';
COMMENT ON COLUMN tracking_events.device IS 'Device brand/model';
COMMENT ON COLUMN tracking_events.device_type IS 'Device type: desktop, mobile, tablet, bot';
COMMENT ON COLUMN tracking_events.email_client IS 'Email client if detected';
COMMENT ON COLUMN tracking_events.is_bot IS 'True if request came from a bot';
COMMENT ON COLUMN tracking_events.is_suspicious IS 'True if request is suspicious but not confirmed bot';
COMMENT ON COLUMN tracking_events.bot_type IS 'Type of bot (search_engine, social_media, security_scanner, etc)';
COMMENT ON COLUMN tracking_events.bot_name IS 'Specific bot name (Googlebot, Barracuda, etc)';
COMMENT ON COLUMN tracking_events.bot_score IS 'Bot risk score 0-100 (higher = more likely bot)';
