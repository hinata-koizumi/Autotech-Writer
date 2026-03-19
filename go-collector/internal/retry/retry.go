package retry

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"time"
)

// RetryableStatusCodes are HTTP status codes that warrant a retry.
var RetryableStatusCodes = map[int]bool{
	http.StatusTooManyRequests:     true, // 429
	http.StatusServiceUnavailable:  true, // 503
	http.StatusBadGateway:          true, // 502
	http.StatusGatewayTimeout:      true, // 504
}

// Config holds retry configuration.
type Config struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
}

// DefaultConfig returns a sensible default retry configuration.
func DefaultConfig() Config {
	return Config{
		MaxRetries: 5,
		BaseDelay:  1 * time.Second,
		MaxDelay:   60 * time.Second,
	}
}

// DoWithRetry executes an HTTP request with exponential backoff retry.
// It retries on retryable status codes (429, 503, etc.)
func DoWithRetry(ctx context.Context, client *http.Client, req *http.Request, cfg Config) (*http.Response, error) {
	var lastErr error

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := calculateBackoff(attempt, cfg.BaseDelay, cfg.MaxDelay)
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("context cancelled during retry backoff: %w", ctx.Err())
			case <-time.After(delay):
			}
		}

		// Clone the request for each attempt (body can only be read once)
		reqClone := req.Clone(ctx)
		resp, err := client.Do(reqClone)
		if err != nil {
			lastErr = fmt.Errorf("attempt %d: %w", attempt+1, err)
			continue
		}

		if !RetryableStatusCodes[resp.StatusCode] {
			return resp, nil
		}

		// Close the body to avoid resource leaks before retry
		resp.Body.Close()
		lastErr = fmt.Errorf("attempt %d: received retryable status %d", attempt+1, resp.StatusCode)
	}

	return nil, fmt.Errorf("all %d retries exhausted: %w", cfg.MaxRetries+1, lastErr)
}

// calculateBackoff calculates the delay for exponential backoff with jitter.
func calculateBackoff(attempt int, baseDelay, maxDelay time.Duration) time.Duration {
	delay := time.Duration(float64(baseDelay) * math.Pow(2, float64(attempt-1)))
	if delay > maxDelay {
		delay = maxDelay
	}
	// Add jitter: 0.5x to 1.5x
	jitter := 0.5 + rand.Float64()
	delay = time.Duration(float64(delay) * jitter)
	return delay
}
