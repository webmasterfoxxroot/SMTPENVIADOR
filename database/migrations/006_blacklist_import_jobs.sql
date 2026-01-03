-- Migration: Add blacklist_import_jobs table
-- This table stores async import jobs for blacklist SQL files

CREATE TABLE IF NOT EXISTS blacklist_import_jobs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    file_name VARCHAR(500) NOT NULL,
    file_path VARCHAR(500),
    db_type VARCHAR(50) NOT NULL, -- mailwizz, newapp, mumara
    status VARCHAR(50) DEFAULT 'pending', -- pending, processing, completed, failed, cancelled
    total_emails INTEGER DEFAULT 0,
    processed INTEGER DEFAULT 0,
    imported INTEGER DEFAULT 0,
    duplicates INTEGER DEFAULT 0,
    errors INTEGER DEFAULT 0,
    error_message TEXT,
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_blacklist_import_jobs_status ON blacklist_import_jobs(status);
