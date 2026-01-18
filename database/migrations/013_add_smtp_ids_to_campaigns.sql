-- Add smtp_ids column to campaigns for selecting specific SMTPs
ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS smtp_ids TEXT DEFAULT '';
