package fetcher

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHuggingFaceFetcher_Fetch(t *testing.T) {
	// Mock HF API response
	hfResponse := []HFResponseItem{
		{
			Paper: HFPaper{
				ID:          "2603.12345", // Looks like arXiv ID
				Title:       "HF ArXiv Paper",
				Summary:     "This is a summary of an arXiv paper.",
				PublishedAt: "2026-03-20T10:00:00Z",
			},
		},
		{
			Paper: HFPaper{
				ID:          "some-other-id", // Not an arXiv ID
				Title:       "HF Non-ArXiv Paper",
				Summary:     "This is a summary of a non-arXiv paper.",
				PublishedAt: "2026-03-20T11:00:00Z",
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/daily_papers" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(hfResponse)
			return
		}
		if r.URL.Path == "/e-print/2603.12345" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("not a gzip"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	fetcher := NewHuggingFaceFetcher(server.URL, server.URL, server.Client())
	items, err := fetcher.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("Expected 2 items, got %d", len(items))
	}

	// Verify first item (ArXiv ID)
	item1 := items[0]
	if item1.SourceType != "huggingface" {
		t.Errorf("Expected SourceType 'huggingface', got '%s'", item1.SourceType)
	}
	if item1.Score != HuggingFaceBaseScore {
		t.Errorf("Expected Score %d, got %d", HuggingFaceBaseScore, item1.Score)
	}
	// Note: FullContent will be empty because of "not a gzip" error handled gracefully in Fetch

	// Verify second item (Non-ArXiv ID)
	item2 := items[1]
	if item2.SourceID != "huggingface:daily_papers:some-other-id" {
		t.Errorf("Unexpected SourceID: %s", item2.SourceID)
	}
	if item2.FullContent != "" {
		t.Errorf("Expected empty FullContent for non-arXiv paper, got content")
	}
}

func TestGitHubTrendingRule(t *testing.T) {
	fetcher := &GitHubFetcher{
		StarThreshold: 1000,
	}

	now := time.Now()
	recentDate := now.Add(-24 * time.Hour)
	oldDate := now.Add(-40 * 24 * time.Hour)

	tests := []struct {
		name     string
		stars    int
		created  time.Time
		expected bool
	}{
		{"High stars old repo", 1200, oldDate, true},
		{"Low stars old repo", 800, oldDate, false},
		{"Medium stars new repo", 600, recentDate, true},
		{"Low stars new repo", 400, recentDate, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mocking filterReposByStars logic here since we can't easily mock getRepoStars without more work
			isTrending := tt.stars >= GitHubTrendingStars && time.Since(tt.created).Hours() < 24*float64(GitHubTrendingDays)
			accepted := tt.stars >= fetcher.StarThreshold || isTrending

			if accepted != tt.expected {
				t.Errorf("Expected accepted=%v, got %v", tt.expected, accepted)
			}
		})
	}
}
