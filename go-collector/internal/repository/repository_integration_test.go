package repository

import (
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/autotech-writer/go-collector/internal/models"
	_ "github.com/lib/pq"
)

func getTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		t.Skip("TEST_DB_DSN not set, skipping integration test")
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	if err := db.Ping(); err != nil {
		t.Fatalf("failed to ping database: %v", err)
	}

	return db
}

// cleanUpTestTable removes test data before or after a test run.
func cleanUpTestTable(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec("DELETE FROM articles WHERE source_type = 'test'")
	if err != nil {
		t.Fatalf("failed to clean up test data: %v", err)
	}
}

func countTestRows(t *testing.T, db *sql.DB, sourceType, sourceID string) int {
	t.Helper()
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM articles WHERE source_type = $1 AND source_id = $2", sourceType, sourceID).Scan(&count)
	if err != nil {
		t.Fatalf("failed to count rows: %v", err)
	}
	return count
}

func TestIntegration_Repository(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	repo := NewRepository(db)
	now := time.Now().Truncate(time.Second)
	baseItem := models.FetchedItem{
		SourceType:  "test",
		SourceID:    "integration-test-1",
		Title:       "Integration Test Paper",
		Summary:     "Validating actual DB insertion",
		URL:         "http://test.org/1",
		PublishedAt: now,
		Score:       100,
		RawData:     `{"integration": true}`,
	}

	t.Run("InsertItem_NewRecord", func(t *testing.T) {
		cleanUpTestTable(t, db) // fresh state
		defer cleanUpTestTable(t, db)

		inserted, err := repo.InsertItem(baseItem)
		if err != nil {
			t.Fatalf("expected no error on first insert, got: %v", err)
		}
		if !inserted {
			t.Error("expected item to be inserted (new), got false")
		}

		if count := countTestRows(t, db, baseItem.SourceType, baseItem.SourceID); count != 1 {
			t.Errorf("expected 1 row in DB, got %d", count)
		}
	})

	t.Run("InsertItem_DuplicateRecord", func(t *testing.T) {
		cleanUpTestTable(t, db) // fresh state
		defer cleanUpTestTable(t, db)

		// Setup: Insert first record
		if _, err := repo.InsertItem(baseItem); err != nil {
			t.Fatalf("setup failed: %v", err)
		}

		// Change title to verify ON CONFLICT DO NOTHING does not update other fields
		duplicateItem := baseItem
		duplicateItem.Title = "Duplicate Integration Test Paper"

		// Execution: Try inserting duplicate
		insertedDuplicate, err := repo.InsertItem(duplicateItem)
		if err != nil {
			t.Fatalf("expected no error on duplicate insert, got: %v", err)
		}
		if insertedDuplicate {
			t.Error("expected item NOT to be inserted (duplicate), got true")
		}

		// Verification: count should still be 1
		if count := countTestRows(t, db, baseItem.SourceType, baseItem.SourceID); count != 1 {
			t.Errorf("expected 1 row in DB after duplicate insert, got %d", count)
		}
	})
}
