-- Migration 004: Create seen_articles table for lightweight deduplication
CREATE TABLE IF NOT EXISTS seen_articles (
    source_id   VARCHAR(512) PRIMARY KEY,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Migrate existing source_ids from articles to seen_articles to avoid duplicates
INSERT INTO seen_articles (source_id, created_at)
SELECT source_id, created_at FROM articles
ON CONFLICT (source_id) DO NOTHING;

-- Index for cleanup of very old seen_articles (e.g. 6 months)
CREATE INDEX IF NOT EXISTS idx_seen_articles_created_at ON seen_articles(created_at);
