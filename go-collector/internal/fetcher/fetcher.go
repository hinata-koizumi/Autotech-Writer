package fetcher

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

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
func fetchURLWithRetry(ctx context.Context, client *http.Client, url string, retry RetryConfig) ([]byte, error) {
	var body []byte
	var err error

	for attempt := 0; attempt <= retry.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := retry.BaseDelay * time.Duration(1<<(attempt-1))
			slog.Warn("Retrying fetch", "url", url, "attempt", attempt, "delay", delay)
			
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		body, err = fetchURL(ctx, client, url)
		if err == nil {
			return body, nil
		}

		// Don't retry on certain errors (e.g. 404)
		if strings.Contains(err.Error(), "404") {
			return nil, err
		}
	}

	return nil, fmt.Errorf("after %d retries: %w", retry.MaxRetries, err)
}

// fetchURL is a helper to perform HTTP GET requests and return the body.
func fetchURL(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "Autotech-Writer/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("performing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned status: %d (%s)", resp.StatusCode, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	return body, nil
}
