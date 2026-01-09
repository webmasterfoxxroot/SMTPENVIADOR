-- Migration: Add from_name column to templates table
-- This allows templates to store the sender name

ALTER TABLE templates ADD COLUMN IF NOT EXISTS from_name VARCHAR(255);

-- Update existing templates with empty from_name
UPDATE templates SET from_name = '' WHERE from_name IS NULL;
