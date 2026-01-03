-- Migration: Add list_ids column to campaigns for multi-list support
-- Run this on existing databases

-- Add list_ids column (TEXT storing comma-separated UUIDs)
ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS list_ids TEXT;

-- Copy existing list_id values to list_ids for backwards compatibility
UPDATE campaigns SET list_ids = list_id::text WHERE list_ids IS NULL AND list_id IS NOT NULL;
