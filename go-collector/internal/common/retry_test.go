package common

import (
	"testing"
	"time"
)

func TestBackoffDelay(t *testing.T) {
	baseDelay := 100 * time.Millisecond
	maxDelay := 1 * time.Second

	// Case 1: First attempt (0)
	// Base * 2^0 + jitter (0 to 0.5 * base)
	// range: [100ms, 150ms]
	delay0 := BackoffDelay(0, baseDelay, maxDelay)
	if delay0 < 100*time.Millisecond || delay0 > 150*time.Millisecond {
		t.Errorf("Attempt 0: expected [100ms, 150ms], got %v", delay0)
	}

	// Case 2: Attempt 1
	// Base * 2^1 + jitter
	// range: [200ms, 250ms]
	delay1 := BackoffDelay(1, baseDelay, maxDelay)
	if delay1 < 200*time.Millisecond || delay1 > 250*time.Millisecond {
		t.Errorf("Attempt 1: expected [200ms, 250ms], got %v", delay1)
	}

	// Case 3: Reaching max delay
	// Base * 2^5 = 3.2s + jitter -> should be clamped by maxDelay (1s)
	delayHigh := BackoffDelay(5, baseDelay, maxDelay)
	if delayHigh != maxDelay {
		t.Errorf("Expected max delay %v, got %v", maxDelay, delayHigh)
	}
}

func TestIsRetryableStatusCode(t *testing.T) {
	tests := []struct {
		code     int
		expected bool
	}{
		{200, false},
		{400, false},
		{404, false},
		{429, true},
		{500, true},
		{502, true},
		{503, true},
		{504, true},
	}

	for _, tc := range tests {
		if got := IsRetryableStatusCode(tc.code); got != tc.expected {
			t.Errorf("IsRetryableStatusCode(%d) = %v; want %v", tc.code, got, tc.expected)
		}
	}
}
