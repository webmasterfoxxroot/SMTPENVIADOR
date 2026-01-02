-- Migration: Add import_jobs table for persistent job tracking
-- Run this on existing databases

CREATE TABLE IF NOT EXISTS import_jobs (
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

CREATE INDEX IF NOT EXISTS idx_import_jobs_list_id ON import_jobs(list_id);
CREATE INDEX IF NOT EXISTS idx_import_jobs_status ON import_jobs(status);

-- Add trigger for updated_at
DROP TRIGGER IF EXISTS update_import_jobs_updated_at ON import_jobs;
CREATE TRIGGER update_import_jobs_updated_at BEFORE UPDATE ON import_jobs FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
