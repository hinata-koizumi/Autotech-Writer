-- Migration 002: Update articles table for triage optimization and X posting robustness

-- Add full_content column for LaTeX/GitHub patches
ALTER TABLE articles ADD COLUMN IF NOT EXISTS full_content TEXT;

-- Add columns for X thread posting tracking
ALTER TABLE articles ADD COLUMN IF NOT EXISTS last_posted_index INTEGER DEFAULT 0;
ALTER TABLE articles ADD COLUMN IF NOT EXISTS x_thread_ids JSONB DEFAULT '[]'::jsonb;

-- Update status constraint/check if it exists, or just ensure we handle new statuses in app logic.
-- PostgreSQL doesn't have a simple way to update CHECK constraints without dropping and recreating.
-- Since status is just VARCHAR(50), we can just use the new values in application code.

COMMENT ON COLUMN articles.full_content IS 'Full technical content (LaTeX/PR diffs) used for fact extraction but not triage.';
COMMENT ON COLUMN articles.last_posted_index IS 'Index of the last successfully posted tweet in the thread.';
COMMENT ON COLUMN articles.x_thread_ids IS 'List of tweet IDs in the thread for resumption.';
