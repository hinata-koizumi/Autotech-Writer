-- Proposed DDL for Autotech Writer Articles table

CREATE TABLE IF NOT EXISTS articles (
    id SERIAL PRIMARY KEY,
    source_type VARCHAR(50) NOT NULL,    -- 'arxiv', 'github', 'rss'
    source_id VARCHAR(255) UNIQUE NOT NULL, -- unique ID from the source (e.g. arXiv ID, RSS link)
    title TEXT NOT NULL,
    summary TEXT,
    url TEXT,
    published_at TIMESTAMP WITH TIME ZONE,
    
    -- Processing State
    status VARCHAR(50) DEFAULT 'pending', -- pending, processing, completed, failed, retry, ignored, rejected_compliance
    retry_count INTEGER DEFAULT 0,
    
    -- Generated Content
    hook_text TEXT,        -- X post hook/short summary (optional)
    article_body TEXT,     -- Long form generated article content (1500-2500 chars)
    x_post_id VARCHAR(100), -- ID from X API after posting
    
    -- Raw data for reference/debugging
    raw_data JSONB,
    
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Index for status-based polling
CREATE INDEX IF NOT EXISTS idx_articles_status ON articles(status);
CREATE INDEX IF NOT EXISTS idx_articles_created_at ON articles(created_at);
