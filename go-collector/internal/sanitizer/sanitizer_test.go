package sanitizer

import (
	"strings"
	"testing"
	"time"

	"github.com/autotech-writer/go-collector/internal/models"
)

// ============================================================
// 異常な入力値のサニタイズ検証
// ============================================================

// [異常系] タイトルが1万文字以上の場合、適切に切り詰められること
func TestSanitize_OversizedTitle(t *testing.T) {
	longTitle := strings.Repeat("あ", 10001)
	item := models.FetchedItem{
		SourceType:  "arxiv",
		SourceID:    "test-001",
		Title:       longTitle,
		Summary:     "Normal summary",
		PublishedAt: time.Now().Add(-24 * time.Hour),
	}

	sanitized, err := Sanitize(item)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	titleRunes := []rune(sanitized.Title)
	// MaxTitleLength (5000) + "..." (3 chars)
	if len(titleRunes) > MaxTitleLength+3 {
		t.Errorf("expected title length <= %d, got %d", MaxTitleLength+3, len(titleRunes))
	}
}

// [異常系] 未来の日付が含まれる場合、現在日付に補正されること
func TestSanitize_FutureDate(t *testing.T) {
	futureDate := time.Now().Add(365 * 24 * time.Hour) // 1 year in the future
	item := models.FetchedItem{
		SourceType:  "arxiv",
		SourceID:    "test-002",
		Title:       "Future Paper",
		Summary:     "From the future",
		PublishedAt: futureDate,
	}

	sanitized, err := Sanitize(item)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if sanitized.PublishedAt.After(time.Now().Add(1 * time.Second)) {
		t.Errorf("expected PublishedAt to be clamped to now, still in future: %v", sanitized.PublishedAt)
	}
}

// [正常系] 過去の正常な日付は変更されないこと
func TestSanitize_PastDate_Unchanged(t *testing.T) {
	pastDate := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	item := models.FetchedItem{
		SourceType:  "arxiv",
		SourceID:    "test-003",
		Title:       "Past Paper",
		Summary:     "Normal date",
		PublishedAt: pastDate,
	}

	sanitized, err := Sanitize(item)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if !sanitized.PublishedAt.Equal(pastDate) {
		t.Errorf("expected past date to be unchanged, got %v", sanitized.PublishedAt)
	}
}

// [異常系] 制御文字が含まれる場合、除去されること
func TestSanitize_ControlCharacters(t *testing.T) {
	item := models.FetchedItem{
		SourceType:  "arxiv",
		SourceID:    "test-004",
		Title:       "Normal\x00Title\x01With\x02Control\x03Chars",
		Summary:     "Summary\x07with\x08bells\x7fand\x1bescapes",
		PublishedAt: time.Now().Add(-24 * time.Hour),
	}

	sanitized, err := Sanitize(item)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Control chars should be removed
	if strings.ContainsAny(sanitized.Title, "\x00\x01\x02\x03") {
		t.Errorf("expected control chars removed from title, got: %q", sanitized.Title)
	}
	if strings.ContainsAny(sanitized.Summary, "\x07\x08\x7f\x1b") {
		t.Errorf("expected control chars removed from summary, got: %q", sanitized.Summary)
	}

	// Content should be preserved (minus control chars)
	if sanitized.Title != "NormalTitleWithControlChars" {
		t.Errorf("expected 'NormalTitleWithControlChars', got '%s'", sanitized.Title)
	}
}

// [正常系] 改行とタブは保持されること
func TestSanitize_PreservesNewlinesAndTabs(t *testing.T) {
	item := models.FetchedItem{
		SourceType:  "arxiv",
		SourceID:    "test-005",
		Title:       "Title\nWith\tFormatting",
		Summary:     "Line1\nLine2\r\nLine3",
		PublishedAt: time.Now().Add(-1 * time.Hour),
	}

	sanitized, err := Sanitize(item)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if !strings.Contains(sanitized.Title, "\n") || !strings.Contains(sanitized.Title, "\t") {
		t.Errorf("expected newlines/tabs preserved in title, got: %q", sanitized.Title)
	}
}

// [異常系] 空のタイトルでもクラッシュしないこと
func TestSanitize_EmptyTitle(t *testing.T) {
	item := models.FetchedItem{
		SourceType:  "arxiv",
		SourceID:    "test-006",
		Title:       "",
		Summary:     "Has summary but no title",
		PublishedAt: time.Now().Add(-1 * time.Hour),
	}

	sanitized, err := Sanitize(item)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if sanitized.Title != "" {
		t.Errorf("expected empty title to remain empty, got '%s'", sanitized.Title)
	}
}

// [異常系] ゼロ値の日付は変更されないこと
func TestSanitize_ZeroDate(t *testing.T) {
	item := models.FetchedItem{
		SourceType:  "arxiv",
		SourceID:    "test-007",
		Title:       "Zero Date Paper",
		Summary:     "No date available",
		PublishedAt: time.Time{}, // zero value
	}

	sanitized, err := Sanitize(item)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if !sanitized.PublishedAt.IsZero() {
		t.Errorf("expected zero date to remain zero, got %v", sanitized.PublishedAt)
	}
}

// [異常系] 文字化けを含むUnicode文字列が安全に処理されること
func TestSanitize_MalformedUnicode(t *testing.T) {
	item := models.FetchedItem{
		SourceType:  "arxiv",
		SourceID:    "test-008",
		Title:       "タイトル with mixed 日本語 and \x00null\x01chars",
		Summary:     "サマリー テスト",
		PublishedAt: time.Now().Add(-1 * time.Hour),
	}

	sanitized, err := Sanitize(item)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Japanese characters should be preserved
	if !strings.Contains(sanitized.Title, "タイトル") {
		t.Errorf("expected Japanese chars preserved, got: %s", sanitized.Title)
	}
	// Control chars should be removed
	if strings.Contains(sanitized.Title, "\x00") || strings.Contains(sanitized.Title, "\x01") {
		t.Errorf("expected control chars removed, got: %q", sanitized.Title)
	}
}
