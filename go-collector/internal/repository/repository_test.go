package repository

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/autotech-writer/go-collector/internal/models"
)

// ============================================================
// DB保存と重複排除の検証
// ============================================================

// [正常系] 新規データをpendingとしてInsertできること
func TestInsertItem_NewItem(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewRepository(db)

	item := models.FetchedItem{
		SourceType:  "arxiv",
		SourceID:    "http://arxiv.org/abs/2401.00001v1",
		Title:       "Test Paper",
		Summary:     "Test summary",
		URL:         "http://arxiv.org/abs/2401.00001v1",
		PublishedAt: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
		RawData:     `{"test": true}`,
	}

	mock.ExpectExec("INSERT INTO articles").
		WithArgs(
			item.SourceType,
			item.SourceID,
			item.Title,
			item.Summary,
			item.URL,
			item.PublishedAt,
			item.RawData,
			sqlmock.AnyArg(), // created_at / updated_at
		).
		WillReturnResult(sqlmock.NewResult(1, 1)) // 1 row affected = new insert

	inserted, err := repo.InsertItem(item)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !inserted {
		t.Error("expected item to be inserted (new), got false")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

// [正常系] 既存のsource_idが存在する場合Unique制約で安全にスキップされること
func TestInsertItem_DuplicateSkip(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewRepository(db)

	item := models.FetchedItem{
		SourceType:  "arxiv",
		SourceID:    "http://arxiv.org/abs/2401.00001v1",
		Title:       "Duplicate Paper",
		Summary:     "Already exists",
		URL:         "http://arxiv.org/abs/2401.00001v1",
		PublishedAt: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
		RawData:     `{}`,
	}

	mock.ExpectExec("INSERT INTO articles").
		WithArgs(
			item.SourceType,
			item.SourceID,
			item.Title,
			item.Summary,
			item.URL,
			item.PublishedAt,
			item.RawData,
			sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(0, 0)) // 0 rows affected = duplicate, ON CONFLICT DO NOTHING

	inserted, err := repo.InsertItem(item)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if inserted {
		t.Error("expected item NOT to be inserted (duplicate), got true")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

// [異常系] DBエラー時にエラーが正しく返されること
func TestInsertItem_DBError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewRepository(db)

	item := models.FetchedItem{
		SourceType:  "arxiv",
		SourceID:    "http://arxiv.org/abs/error-test",
		Title:       "Error Paper",
		Summary:     "This should fail",
		URL:         "http://arxiv.org/abs/error-test",
		PublishedAt: time.Now(),
		RawData:     `{}`,
	}

	mock.ExpectExec("INSERT INTO articles").
		WithArgs(
			item.SourceType,
			item.SourceID,
			item.Title,
			item.Summary,
			item.URL,
			item.PublishedAt,
			item.RawData,
			sqlmock.AnyArg(),
		).
		WillReturnError(sqlmock.ErrCancelled)

	_, err = repo.InsertItem(item)
	if err == nil {
		t.Fatal("expected error when DB fails, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}
