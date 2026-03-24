package models

import "time"

// FetchedItem represents a single item collected from any data source.
type FetchedItem struct {
	SourceType     string            `json:"source_type"` // "arxiv", "github", "rss"
	SourceID       string            `json:"source_id"`   // Unique identifier (arXiv ID, release URL hash, etc.)
	Title          string            `json:"title"`
	Summary        string            `json:"summary"`
	FullContent    string            `json:"full_content,omitempty"`
	URL            string            `json:"url"`
	PublishedAt    time.Time         `json:"published_at"`
	Score          int               `json:"score"`
	IsBreakingNews bool              `json:"is_breaking_news"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	RawData        string            `json:"raw_data"` // JSON blob
}
