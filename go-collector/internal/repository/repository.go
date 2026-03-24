package repository

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/autotech-writer/go-collector/internal/models"
)

// Repository handles database operations for articles.
type Repository struct {
	DB *sql.DB
}

// NewRepository creates a new Repository.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{DB: db}
}

// ItemExists checks if an item with the given source_id has been seen before.
func (r *Repository) ItemExists(sourceID string) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM seen_articles WHERE source_id = $1)`
	err := r.DB.QueryRow(query, sourceID).Scan(&exists)
	return exists, err
}

// InsertItem inserts a new article for processing and marks it as seen.
// Returns true if inserted into processing, false if duplicate or skipped (0 score).
func (r *Repository) InsertItem(item models.FetchedItem) (bool, error) {
	tx, err := r.DB.Begin()
	if err != nil {
		return false, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	// 1. Mark as seen (deduplication)
	_, err = tx.Exec(`INSERT INTO seen_articles (source_id, created_at) VALUES ($1, NOW()) ON CONFLICT DO NOTHING`, item.SourceID)
	if err != nil {
		return false, fmt.Errorf("marking as seen: %w", err)
	}

	// 2. If score is 0, we don't store the content for processing, just keep it in seen_articles
	if item.Score <= 0 {
		if err := tx.Commit(); err != nil {
			return false, err
		}
		return false, nil
	}

	// 3. Store in articles for LLM processing
	query := `
		INSERT INTO articles (source_type, source_id, title, summary, full_content, url, published_at, raw_data, status, score, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'pending', $9, $10, $10)
		ON CONFLICT (source_id) DO NOTHING
	`

	now := time.Now()
	result, err := tx.Exec(query,
		item.SourceType,
		item.SourceID,
		item.Title,
		item.Summary,
		item.FullContent,
		item.URL,
		item.PublishedAt,
		item.RawData,
		item.Score,
		now,
	)
	if err != nil {
		return false, fmt.Errorf("inserting article for processing: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("committing transaction: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	return rowsAffected > 0, nil
}

// PruneOldArticles deletes articles in the processing table older than the specified days.
func (r *Repository) PruneOldArticles(days int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -days)
	result, err := r.DB.Exec(`DELETE FROM articles WHERE created_at < $1`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("pruning articles: %w", err)
	}
	return result.RowsAffected()
}

// PruneSeenArticles deletes history from seen_articles older than the specified days.
func (r *Repository) PruneSeenArticles(days int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -days)
	result, err := r.DB.Exec(`DELETE FROM seen_articles WHERE created_at < $1`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("pruning seen articles: %w", err)
	}
	return result.RowsAffected()
}

// RecordStats logs the results of a collection cycle for a specific source.
func (r *Repository) RecordStats(source string, fetched, skipped, errorCount int) error {
	query := `
		INSERT INTO collection_stats (source_name, fetched_count, skipped_count, error_count)
		VALUES ($1, $2, $3, $4)
	`
	_, err := r.DB.Exec(query, source, fetched, skipped, errorCount)
	if err != nil {
		return fmt.Errorf("recording collection stats: %w", err)
	}
	return nil
}
