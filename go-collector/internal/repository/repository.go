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

// InsertItem inserts a new article into the database.
// Returns true if inserted, false if duplicate (already exists).
func (r *Repository) InsertItem(item models.FetchedItem) (bool, error) {
	query := `
		INSERT INTO articles (source_type, source_id, title, summary, url, published_at, raw_data, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'pending', $8, $8)
		ON CONFLICT (source_id) DO NOTHING
	`

	now := time.Now()
	result, err := r.DB.Exec(query,
		item.SourceType,
		item.SourceID,
		item.Title,
		item.Summary,
		item.URL,
		item.PublishedAt,
		item.RawData,
		now,
	)
	if err != nil {
		return false, fmt.Errorf("inserting item %s: %w", item.SourceID, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("checking rows affected: %w", err)
	}

	return rowsAffected > 0, nil
}
