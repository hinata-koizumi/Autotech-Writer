-- Migration 003: Add score column and indices for optimization
ALTER TABLE articles ADD COLUMN IF NOT EXISTS score INTEGER DEFAULT 0;

-- Indices for cleanup performance
CREATE INDEX IF NOT EXISTS idx_articles_score ON articles(score);
CREATE INDEX IF NOT EXISTS idx_articles_created_at ON articles(created_at);
