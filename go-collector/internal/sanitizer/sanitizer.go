package sanitizer

import (
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/autotech-writer/go-collector/internal/common"
	"github.com/autotech-writer/go-collector/internal/models"
)

const (
	// MaxTitleLength is the maximum allowed length for a title.
	MaxTitleLength = 500
	// MaxSummaryLength is the maximum allowed length for a summary.
	MaxSummaryLength = 200000

	truncationSuffix = "..."
)

// englishNGRe is a compiled word-boundary regex for English NG keywords.
// Built once at package init to avoid recompilation on every Sanitize call.
var englishNGRe *regexp.Regexp

func init() {
	pattern := `(?i)\b(` + strings.Join(common.EnglishNGKeywords, "|") + `)\b`
	englishNGRe = regexp.MustCompile(pattern)
}

// Sanitize cleans and validates a FetchedItem, returning a sanitized copy.
// Items that cannot be salvaged return an error.
func Sanitize(item models.FetchedItem) (models.FetchedItem, error) {
	// Reject items with no title
	if strings.TrimSpace(item.Title) == "" {
		return item, errors.New("empty title")
	}

	// Compliance check
	if containsNGWord(item.Title) || containsNGWord(item.Summary) {
		return item, errors.New("NG keyword detected")
	}

	// Remove control characters from title and summary
	item.Title = removeControlChars(item.Title)
	item.Summary = removeControlChars(item.Summary)

	// Truncate oversized title
	if titleRunes := []rune(item.Title); len(titleRunes) > MaxTitleLength {
		item.Title = string(titleRunes[:MaxTitleLength]) + truncationSuffix
	}

	// Truncate oversized summary
	if summaryRunes := []rune(item.Summary); len(summaryRunes) > MaxSummaryLength {
		item.Summary = string(summaryRunes[:MaxSummaryLength]) + truncationSuffix
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

func containsNGWord(text string) bool {
	lowerText := strings.ToLower(text)

	for _, kw := range common.JapaneseNGKeywords {
		if strings.Contains(lowerText, kw) {
			return true
		}
	}

	return englishNGRe.MatchString(lowerText)
}
