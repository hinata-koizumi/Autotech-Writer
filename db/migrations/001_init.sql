-- Autotech Writer: PostgreSQL Schema

CREATE TABLE IF NOT EXISTS articles (
    id              BIGSERIAL PRIMARY KEY,
    source_type     VARCHAR(20) NOT NULL,         -- 'arxiv' | 'github' | 'rss'
    source_id       VARCHAR(512) NOT NULL UNIQUE,  -- arXiv ID, release URL hash, etc.
    title           TEXT NOT NULL,
    summary         TEXT,
    url             TEXT,
    published_at    TIMESTAMPTZ,
    raw_data        JSONB,
    status          VARCHAR(30) NOT NULL DEFAULT 'pending',
                    -- pending → processing → completed / failed / ignored / rejected_compliance / retry
    retry_count     INT NOT NULL DEFAULT 0,
    hook_text       TEXT,
    article_body    TEXT,
    x_post_id       TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_articles_status ON articles(status);
CREATE INDEX IF NOT EXISTS idx_articles_source_type ON articles(source_type);
