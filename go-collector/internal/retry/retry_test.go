package retry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// [正常系] 最初のリクエストが成功すればリトライしないこと
func TestDoWithRetry_ImmediateSuccess(t *testing.T) {
	var callCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := Config{MaxRetries: 3, BaseDelay: 10 * time.Millisecond, MaxDelay: 100 * time.Millisecond}
	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	resp, err := DoWithRetry(context.Background(), server.Client(), req, cfg)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}
}

// [異常系] 429→リトライ後成功
func TestDoWithRetry_429ThenSuccess(t *testing.T) {
	var callCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&callCount, 1)
		if count <= 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := Config{MaxRetries: 5, BaseDelay: 10 * time.Millisecond, MaxDelay: 100 * time.Millisecond}
	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	resp, err := DoWithRetry(context.Background(), server.Client(), req, cfg)
	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	defer resp.Body.Close()
	if atomic.LoadInt32(&callCount) != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
}

// [異常系] 503→リトライ後成功
func TestDoWithRetry_503ThenSuccess(t *testing.T) {
	var callCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&callCount, 1)
		if count <= 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := Config{MaxRetries: 3, BaseDelay: 10 * time.Millisecond, MaxDelay: 100 * time.Millisecond}
	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	resp, err := DoWithRetry(context.Background(), server.Client(), req, cfg)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	defer resp.Body.Close()
}

// [異常系] 全リトライ回数を使い切った
func TestDoWithRetry_AllRetriesExhausted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	cfg := Config{MaxRetries: 2, BaseDelay: 10 * time.Millisecond, MaxDelay: 50 * time.Millisecond}
	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	_, err := DoWithRetry(context.Background(), server.Client(), req, cfg)
	if err == nil {
		t.Fatal("expected error when all retries exhausted")
	}
}

// [異常系] コンテキストキャンセルでリトライ中断
func TestDoWithRetry_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	cfg := Config{MaxRetries: 10, BaseDelay: 100 * time.Millisecond, MaxDelay: 1 * time.Second}
	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	_, err := DoWithRetry(ctx, server.Client(), req, cfg)
	if err == nil {
		t.Fatal("expected error due to context cancellation")
	}
}

// [正常系] 非リトライ対象(400)はそのまま返る
func TestDoWithRetry_NonRetryableStatus(t *testing.T) {
	var callCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	cfg := Config{MaxRetries: 3, BaseDelay: 10 * time.Millisecond, MaxDelay: 100 * time.Millisecond}
	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	resp, err := DoWithRetry(context.Background(), server.Client(), req, cfg)
	if err != nil {
		t.Fatalf("expected no error for non-retryable, got: %v", err)
	}
	defer resp.Body.Close()
	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("expected 1 call (no retry for 400), got %d", callCount)
	}
}

// [正常系] バックオフが正の値を返すこと
func TestCalculateBackoff_Positive(t *testing.T) {
	for attempt := 1; attempt <= 5; attempt++ {
		d := calculateBackoff(attempt, 100*time.Millisecond, 10*time.Second)
		if d <= 0 {
			t.Errorf("attempt %d: expected positive delay, got %v", attempt, d)
		}
	}
}
