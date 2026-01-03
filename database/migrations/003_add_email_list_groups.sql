-- Migration: Add email_list_groups table for organizing lists
-- Run this on existing databases

-- Create groups table
CREATE TABLE IF NOT EXISTS email_list_groups (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    color VARCHAR(7) DEFAULT '#3B82F6', -- hex color for UI
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Add group_id column to email_lists
ALTER TABLE email_lists ADD COLUMN IF NOT EXISTS group_id UUID REFERENCES email_list_groups(id) ON DELETE SET NULL;

-- Create index for faster lookups
CREATE INDEX IF NOT EXISTS idx_email_lists_group_id ON email_lists(group_id);

-- Add trigger for updated_at
DROP TRIGGER IF EXISTS update_email_list_groups_updated_at ON email_list_groups;
CREATE TRIGGER update_email_list_groups_updated_at BEFORE UPDATE ON email_list_groups FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
