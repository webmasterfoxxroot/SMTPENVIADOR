-- Migration: Add threads configuration per campaign
-- This allows each campaign to have its own number of parallel workers

-- Add threads column to campaigns (default 10 for backward compatibility)
ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS threads INTEGER DEFAULT 10;

-- Add comment explaining the column
COMMENT ON COLUMN campaigns.threads IS 'Number of parallel threads/workers for this campaign (1-100)';

-- Update existing campaigns to use default threads
UPDATE campaigns SET threads = 10 WHERE threads IS NULL;
