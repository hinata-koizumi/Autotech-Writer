package sanitizer

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// realFetchURL is used for integration testing to fetch actual data from arXiv.
func realFetchURL(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	var body []byte
	var err error

	for attempt := 1; attempt <= 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "Autotech-Writer-Test/1.0")

		resp, err := client.Do(req)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return io.ReadAll(resp.Body)
			}
			err = fmt.Errorf("server returned status: %d", resp.StatusCode)
		}

		if attempt < 3 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
	}
	return nil, err
}

func TestProcessArxivLatex_Integration(t *testing.T) {
	// Skip integration test in CI or short mode unless explicitly requested
	if testing.Short() || os.Getenv("CI") != "" {
		t.Skip("skipping integration test in CI or short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client := &http.Client{
		Timeout: 45 * time.Second,
		Transport: &http.Transport{
			ForceAttemptHTTP2: false,
		},
	}
	baseURL := "https://export.arxiv.org"

	// Segment Anything: 2304.02643
	entryID := "2304.02643"

	t.Logf("Fetching LaTeX for %s...", entryID)
	tex, err := ProcessArxivLatex(ctx, client, baseURL, entryID, realFetchURL)
	if err != nil {
		t.Fatalf("ProcessArxivLatex failed: %v", err)
	}

	if tex == "" {
		t.Fatal("Extracted LaTeX is empty")
	}

	t.Logf("Extracted LaTeX length: %d", len(tex))

	// Verify content
	contentLower := strings.ToLower(tex)
	if len(tex) > 1000 {
		t.Logf("First 500 chars: %s", tex[:500])
	} else {
		t.Logf("Full content: %s", tex)
	}

	// FlashAttention-3 (2407.02071) often uses "FlashAttention" or "Flash Attention"
	// Also check for "Hopper"
	keywords := []string{"abstract", "introduction", "attention", "gpu"}
	for _, kw := range keywords {
		if !strings.Contains(contentLower, kw) {
			t.Errorf("Extracted LaTeX missing keyword: %s", kw)
		}
	}

	// Verify macro expansion (if applicable)
	if !strings.Contains(tex, "\\section") {
		t.Error("Extracted LaTeX missing \\section")
	}

	t.Log("Integration test verification complete")
}
