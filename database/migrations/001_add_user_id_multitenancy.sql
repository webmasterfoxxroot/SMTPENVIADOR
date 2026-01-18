-- Migration: Add user_id to all tables for multi-tenancy
-- Each user will only see their own data

-- Add user_id to smtp_servers
ALTER TABLE smtp_servers ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES users(id) ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS idx_smtp_servers_user_id ON smtp_servers(user_id);

-- Add user_id to email_list_groups
ALTER TABLE email_list_groups ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES users(id) ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS idx_email_list_groups_user_id ON email_list_groups(user_id);

-- Add user_id to email_lists
ALTER TABLE email_lists ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES users(id) ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS idx_email_lists_user_id ON email_lists(user_id);

-- Add user_id to templates
ALTER TABLE templates ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES users(id) ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS idx_templates_user_id ON templates(user_id);

-- Add user_id to campaigns
ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES users(id) ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS idx_campaigns_user_id ON campaigns(user_id);

-- Add user_id to blacklist
ALTER TABLE blacklist ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES users(id) ON DELETE CASCADE;
DROP INDEX IF EXISTS idx_blacklist_email;
CREATE INDEX IF NOT EXISTS idx_blacklist_user_email ON blacklist(user_id, email);

-- Add user_id to warmup tables (if they exist)
DO $$
BEGIN
    -- warmup_smtps
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'warmup_smtps') THEN
        ALTER TABLE warmup_smtps ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES users(id) ON DELETE CASCADE;
        CREATE INDEX IF NOT EXISTS idx_warmup_smtps_user_id ON warmup_smtps(user_id);
    END IF;

    -- warmup_seeds
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'warmup_seeds') THEN
        ALTER TABLE warmup_seeds ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES users(id) ON DELETE CASCADE;
        CREATE INDEX IF NOT EXISTS idx_warmup_seeds_user_id ON warmup_seeds(user_id);
    END IF;

    -- warmup_templates
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'warmup_templates') THEN
        ALTER TABLE warmup_templates ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES users(id) ON DELETE CASCADE;
        CREATE INDEX IF NOT EXISTS idx_warmup_templates_user_id ON warmup_templates(user_id);
    END IF;

    -- warmup_settings
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'warmup_settings') THEN
        ALTER TABLE warmup_settings ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES users(id) ON DELETE CASCADE;
    END IF;

    -- warmup_activity
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'warmup_activity') THEN
        ALTER TABLE warmup_activity ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES users(id) ON DELETE CASCADE;
        CREATE INDEX IF NOT EXISTS idx_warmup_activity_user_id ON warmup_activity(user_id);
    END IF;
END $$;

-- Set existing data to admin user (first user)
DO $$
DECLARE
    admin_id UUID;
BEGIN
    SELECT id INTO admin_id FROM users WHERE role = 'admin' LIMIT 1;

    IF admin_id IS NOT NULL THEN
        UPDATE smtp_servers SET user_id = admin_id WHERE user_id IS NULL;
        UPDATE email_list_groups SET user_id = admin_id WHERE user_id IS NULL;
        UPDATE email_lists SET user_id = admin_id WHERE user_id IS NULL;
        UPDATE templates SET user_id = admin_id WHERE user_id IS NULL;
        UPDATE campaigns SET user_id = admin_id WHERE user_id IS NULL;
        UPDATE blacklist SET user_id = admin_id WHERE user_id IS NULL;

        -- Warmup tables
        IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'warmup_smtps') THEN
            EXECUTE 'UPDATE warmup_smtps SET user_id = $1 WHERE user_id IS NULL' USING admin_id;
        END IF;
        IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'warmup_seeds') THEN
            EXECUTE 'UPDATE warmup_seeds SET user_id = $1 WHERE user_id IS NULL' USING admin_id;
        END IF;
        IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'warmup_templates') THEN
            EXECUTE 'UPDATE warmup_templates SET user_id = $1 WHERE user_id IS NULL' USING admin_id;
        END IF;
        IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'warmup_settings') THEN
            EXECUTE 'UPDATE warmup_settings SET user_id = $1 WHERE user_id IS NULL' USING admin_id;
        END IF;
        IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'warmup_activity') THEN
            EXECUTE 'UPDATE warmup_activity SET user_id = $1 WHERE user_id IS NULL' USING admin_id;
        END IF;
    END IF;
END $$;
