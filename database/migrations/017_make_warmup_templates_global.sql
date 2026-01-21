-- Migration 017: Make existing warmup templates global
-- This migration converts user-specific warmup templates to global templates
-- so they are visible to all users (admin-managed)

-- Convert all existing warmup templates to global (user_id = NULL)
UPDATE warmup_templates
SET user_id = NULL
WHERE user_id IS NOT NULL;

-- Add comment for clarity
COMMENT ON TABLE warmup_templates IS 'Warmup email templates - global templates managed by admin (user_id = NULL)';
