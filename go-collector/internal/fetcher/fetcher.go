package fetcher

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/autotech-writer/go-collector/internal/common"
	"github.com/autotech-writer/go-collector/internal/models"
)

const (
	DefaultArxivMaxResults = 500
	DefaultGitHubPerPage   = 5
	DefaultHTTPTimeout     = 30 * time.Second

	// Semantic Scholar Defaults
	DefaultS2BatchSize = 500
	DefaultS2MinScore  = 10
)

// RetryConfig defines the strategy for retrying failed requests.
type RetryConfig struct {
	MaxRetries int
	BaseDelay  time.Duration
}

var DefaultRetryConfig = RetryConfig{
	MaxRetries: 3,
	BaseDelay:  1 * time.Second,
}

// Fetcher defines the interface for data source fetchers.
type Fetcher interface {
	Fetch(ctx context.Context) ([]models.FetchedItem, error)
	Name() string
}

// --- Helpers ---

// collectFromSources is a helper to fetch from multiple URLs and parse them concurrently.
func collectFromSources(
	ctx context.Context,
	client *http.Client,
	sources []string,
	urls []string,
	parseFunc func([]byte, string) ([]models.FetchedItem, error),
) ([]models.FetchedItem, error) {
	var allItems []models.FetchedItem
	var mu sync.Mutex
	var wg sync.WaitGroup
	var firstErr error
	var errOnce sync.Once

	for i, source := range sources {
		url := urls[i]
		wg.Add(1)
		go func(source, url string) {
			defer wg.Done()

			body, err := fetchURLWithRetry(ctx, client, url, DefaultRetryConfig)
			if err != nil {
				slog.Error("Failed to fetch source", "source", source, "url", url, "error", err)
				errOnce.Do(func() { firstErr = err })
				return
			}

			items, err := parseFunc(body, source)
			if err != nil {
				slog.Error("Failed to parse source", "source", source, "error", err)
				errOnce.Do(func() { firstErr = err })
				return
			}

			mu.Lock()
			allItems = append(allItems, items...)
			mu.Unlock()
		}(source, url)
	}

	wg.Wait()
	return allItems, firstErr
}

// fetchURLWithRetry performs an HTTP GET request with exponential backoff retries.
// Retries on network errors and retryable HTTP status codes (429, 5xx).
func fetchURLWithRetry(ctx context.Context, client *http.Client, url string, retry RetryConfig) ([]byte, error) {
	var lastErr error

	for attempt := 0; attempt <= retry.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := common.BackoffDelay(attempt-1, retry.BaseDelay, 30*time.Second)
			slog.Warn("Retrying fetch", "url", url, "attempt", attempt, "delay", delay)

			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		body, statusCode, err := fetchURLRaw(ctx, client, url)
		if err != nil {
			lastErr = err
			// Network-level errors are always retryable
			continue
		}

		// Non-retryable error status codes (e.g. 404, 400, 403)
		if statusCode != http.StatusOK && !common.IsRetryableStatusCode(statusCode) {
			return nil, fmt.Errorf("server returned non-retryable status: %d", statusCode)
		}

		// Retryable status code (429, 5xx)
		if statusCode != http.StatusOK {
			lastErr = fmt.Errorf("server returned retryable status: %d", statusCode)
			slog.Warn("Retryable HTTP status", "url", url, "status", statusCode)
			continue
		}

		return body, nil
	}

	return nil, fmt.Errorf("after %d retries: %w", retry.MaxRetries, lastErr)
}

// fetchURLRaw performs an HTTP GET and returns body, status code, and error.
// Network errors return (nil, 0, err). HTTP errors return (nil, statusCode, nil).
func fetchURLRaw(ctx context.Context, client *http.Client, url string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "Autotech-Writer/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("performing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("reading response body: %w", err)
	}

	return body, http.StatusOK, nil
}

// fetchURL is a backwards-compatible wrapper for fetchURLRaw that matches
// the func(context.Context, *http.Client, string) ([]byte, error) signature
// used by sanitizer.ProcessArxivLatex.
func fetchURL(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	body, statusCode, err := fetchURLRaw(ctx, client, url)
	if err != nil {
		return nil, err
	}
	if statusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned status: %d", statusCode)
	}
	return body, nil
}
