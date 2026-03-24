-- Migration 005: Create collection_stats table for monitoring data collection health
CREATE TABLE IF NOT EXISTS collection_stats (
    id            SERIAL PRIMARY KEY,
    source_name   VARCHAR(100) NOT NULL,
    fetched_count INTEGER NOT NULL DEFAULT 0,
    skipped_count INTEGER NOT NULL DEFAULT 0,
    error_count   INTEGER NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for analytics over time
CREATE INDEX IF NOT EXISTS idx_collection_stats_source_at ON collection_stats(source_name, created_at);
