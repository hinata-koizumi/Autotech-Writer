package sanitizer

import (
	"strings"
	"time"
	"unicode"

	"github.com/autotech-writer/go-collector/internal/models"
)

const (
	// MaxTitleLength is the maximum allowed length for a title.
	MaxTitleLength = 5000
	// MaxSummaryLength is the maximum allowed length for a summary.
	MaxSummaryLength = 50000
)

// Sanitize cleans and validates a FetchedItem, returning a sanitized copy.
// Items that cannot be salvaged return an error.
func Sanitize(item models.FetchedItem) (models.FetchedItem, error) {
	// Remove control characters from title and summary
	item.Title = removeControlChars(item.Title)
	item.Summary = removeControlChars(item.Summary)

	// Truncate oversized title
	if len([]rune(item.Title)) > MaxTitleLength {
		runes := []rune(item.Title)
		item.Title = string(runes[:MaxTitleLength]) + "..."
	}

	// Truncate oversized summary
	if len([]rune(item.Summary)) > MaxSummaryLength {
		runes := []rune(item.Summary)
		item.Summary = string(runes[:MaxSummaryLength]) + "..."
	}

	// Fix future dates — clamp to current time
	now := time.Now()
	if !item.PublishedAt.IsZero() && item.PublishedAt.After(now) {
		item.PublishedAt = now
	}

	// Trim whitespace
	item.Title = strings.TrimSpace(item.Title)
	item.Summary = strings.TrimSpace(item.Summary)

	return item, nil
}

// removeControlChars removes non-printable control characters from a string,
// preserving newlines, tabs, and standard whitespace.
func removeControlChars(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return r
		}
		if unicode.IsControl(r) {
			return -1 // drop
		}
		return r
	}, s)
}
