package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/autotech-writer/go-collector/internal/models"
	"github.com/autotech-writer/go-collector/internal/sanitizer"
)

var arxivIDRegex = regexp.MustCompile(`^\d{4}\.\d{4,5}(v\d+)?$`)

// HFResponseItem represents a single item in the Hugging Face Daily Papers API response.
type HFResponseItem struct {
	Paper HFPaper `json:"paper"`
}

// HFPaper represents the paper details in the HF API.
type HFPaper struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Summary     string `json:"summary"`
	PublishedAt string `json:"publishedAt"`
}

// HuggingFaceFetcher fetches daily papers from Hugging Face.
type HuggingFaceFetcher struct {
	BaseURL    string
	ArxivURL   string
	HTTPClient *http.Client
}

// NewHuggingFaceFetcher creates a new HuggingFaceFetcher.
func NewHuggingFaceFetcher(baseURL, arxivURL string, client *http.Client) *HuggingFaceFetcher {
	if baseURL == "" {
		baseURL = "https://huggingface.co"
	}
	if arxivURL == "" {
		arxivURL = "https://export.arxiv.org"
	}
	if client == nil {
		client = &http.Client{Timeout: DefaultHTTPTimeout}
	}
	return &HuggingFaceFetcher{
		BaseURL:    baseURL,
		ArxivURL:   arxivURL,
		HTTPClient: client,
	}
}

// Fetch retrieves today's trending papers from Hugging Face.
func (f *HuggingFaceFetcher) Fetch(ctx context.Context) ([]models.FetchedItem, error) {
	url := fmt.Sprintf("%s/api/daily_papers", f.BaseURL)
	
	body, err := fetchURLWithRetry(ctx, f.HTTPClient, url, DefaultRetryConfig)
	if err != nil {
		return nil, fmt.Errorf("fetching HF daily papers: %w", err)
	}

	var hfItems []HFResponseItem
	if err := json.Unmarshal(body, &hfItems); err != nil {
		return nil, fmt.Errorf("unmarshaling HF JSON: %w", err)
	}

	var items []models.FetchedItem
	for _, hfItem := range hfItems {
		p := hfItem.Paper
		if p.ID == "" {
			continue
		}

		publishedAt, _ := time.Parse(time.RFC3339, p.PublishedAt)
		sourceID := fmt.Sprintf("huggingface:daily_papers:%s", p.ID)
		
		item := models.FetchedItem{
			SourceType:  "huggingface",
			SourceID:    sourceID,
			Title:       p.Title,
			Summary:     p.Summary,
			URL:         fmt.Sprintf("https://huggingface.co/papers/%s", p.ID),
			PublishedAt: publishedAt,
			Score:       HuggingFaceBaseScore, // Unconditionally assign base score
			Metadata:    make(map[string]string),
		}

		// Integration with arXiv LaTeX processing
		if arxivIDRegex.MatchString(p.ID) {
			tex, texErr := sanitizer.ProcessArxivLatex(ctx, f.HTTPClient, f.ArxivURL, p.ID, fetchURL)
			if texErr != nil {
				slog.Debug("Failed to fetch/process LaTeX for HF paper", "arxiv_id", p.ID, "error", texErr)
			} else if tex != "" {
				item.FullContent = tex
			}
		}

		items = append(items, item)
	}

	return items, nil
}

func (f *HuggingFaceFetcher) Name() string {
	return "Hugging Face Daily Papers"
}
