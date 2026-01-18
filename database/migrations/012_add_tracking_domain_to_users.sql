-- Add tracking_domain column to users table
ALTER TABLE users ADD COLUMN IF NOT EXISTS tracking_domain VARCHAR(500);
