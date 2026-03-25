package common

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"net/http"
	"time"
)

// RetryableHTTPStatusCodes defines HTTP status codes that should trigger a retry.
var RetryableHTTPStatusCodes = map[int]bool{
	http.StatusTooManyRequests:     true, // 429
	http.StatusInternalServerError: true, // 500
	http.StatusBadGateway:          true, // 502
	http.StatusServiceUnavailable:  true, // 503
	http.StatusGatewayTimeout:      true, // 504
}

// RetryConfig holds configuration for exponential backoff retries.
type RetryConfig struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
}

// DefaultRetryConfig returns a sensible default retry configuration.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries: 3,
		BaseDelay:  1 * time.Second,
		MaxDelay:   30 * time.Second,
	}
}

// RetryableError indicates that the operation can be retried.
type RetryableError struct {
	StatusCode int
	Err        error
}

func (e *RetryableError) Error() string {
	return fmt.Sprintf("retryable error (status %d): %v", e.StatusCode, e.Err)
}

func (e *RetryableError) Unwrap() error {
	return e.Err
}

// IsRetryableStatusCode checks if a status code should trigger a retry.
func IsRetryableStatusCode(statusCode int) bool {
	return RetryableHTTPStatusCodes[statusCode]
}

// DoWithRetry executes an HTTP request function with exponential backoff.
// The requestFunc should return an *http.Response and error.
// If the response has a retryable status code, it will be retried.
func DoWithRetry(ctx context.Context, cfg RetryConfig, requestFunc func() (*http.Response, error)) (*http.Response, error) {
	var lastErr error

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		resp, err := requestFunc()
		if err != nil {
			lastErr = err
			// Network-level errors are retryable
			if attempt < cfg.MaxRetries {
				delay := BackoffDelay(attempt, cfg.BaseDelay, cfg.MaxDelay)
				slog.Warn("Request failed, retrying",
					"attempt", attempt+1,
					"max_retries", cfg.MaxRetries,
					"delay", delay,
					"error", err,
				)
				sleep(ctx, delay)
				continue
			}
			return nil, fmt.Errorf("request failed after %d retries: %w", cfg.MaxRetries, err)
		}

		// Check for retryable HTTP status codes
		if IsRetryableStatusCode(resp.StatusCode) {
			lastErr = &RetryableError{StatusCode: resp.StatusCode, Err: fmt.Errorf("HTTP %d", resp.StatusCode)}
			resp.Body.Close()

			if attempt < cfg.MaxRetries {
				delay := BackoffDelay(attempt, cfg.BaseDelay, cfg.MaxDelay)
				slog.Warn("Retryable HTTP status, retrying",
					"status", resp.StatusCode,
					"attempt", attempt+1,
					"max_retries", cfg.MaxRetries,
					"delay", delay,
				)
				sleep(ctx, delay)
				continue
			}
			return nil, fmt.Errorf("request failed after %d retries: %w", cfg.MaxRetries, lastErr)
		}

		return resp, nil
	}

	return nil, fmt.Errorf("request failed after %d retries: %w", cfg.MaxRetries, lastErr)
}

// BackoffDelay calculates exponential backoff with jitter.
func BackoffDelay(attempt int, baseDelay, maxDelay time.Duration) time.Duration {
	delay := time.Duration(float64(baseDelay) * math.Pow(2, float64(attempt)))
	// Add jitter: 0-50% of base delay
	jitter := time.Duration(rand.Float64() * float64(baseDelay) * 0.5)
	delay += jitter
	if delay > maxDelay {
		delay = maxDelay
	}
	return delay
}

// sleep waits for the given duration or until context is cancelled.
func sleep(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
