package fetcher

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/autotech-writer/go-collector/internal/models"
	"github.com/mmcdole/gofeed"
)

// Fetcher defines the interface for data source fetchers.
type Fetcher interface {
	Fetch(ctx context.Context) ([]models.FetchedItem, error)
}

// --- arXiv ---

// ArxivFeed represents the Atom feed structure from arXiv API.
type ArxivFeed struct {
	XMLName xml.Name     `xml:"feed"`
	Entries []ArxivEntry `xml:"entry"`
}

// ArxivEntry represents a single entry in the arXiv Atom feed.
type ArxivEntry struct {
	ID        string       `xml:"id"`
	Title     string       `xml:"title"`
	Summary   string       `xml:"summary"`
	Published string       `xml:"published"`
	Links     []ArxivLink  `xml:"link"`
	Authors   []ArxivAuthor `xml:"author"`
}

// ArxivLink represents a link element in the arXiv feed.
type ArxivLink struct {
	Href string `xml:"href,attr"`
	Type string `xml:"type,attr"`
	Rel  string `xml:"rel,attr"`
}

// ArxivAuthor represents an author element.
type ArxivAuthor struct {
	Name string `xml:"name"`
}

// ArxivFetcher fetches papers from the arXiv API.
type ArxivFetcher struct {
	BaseURL    string
	Categories []string
	HTTPClient *http.Client
}

// NewArxivFetcher creates a new ArxivFetcher.
func NewArxivFetcher(baseURL string, categories []string, client *http.Client) *ArxivFetcher {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &ArxivFetcher{
		BaseURL:    baseURL,
		Categories: categories,
		HTTPClient: client,
	}
}

// Fetch retrieves the latest papers from arXiv.
func (f *ArxivFetcher) Fetch(ctx context.Context) ([]models.FetchedItem, error) {
	var allItems []models.FetchedItem

	for _, cat := range f.Categories {
		url := fmt.Sprintf("%s/api/query?search_query=cat:%s&sortBy=submittedDate&sortOrder=descending&max_results=10", f.BaseURL, cat)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("creating request for category %s: %w", cat, err)
		}

		resp, err := f.HTTPClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetching arXiv category %s: %w", cat, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("arXiv API returned status %d for category %s", resp.StatusCode, cat)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("reading response body: %w", err)
		}

		items, err := ParseArxivXML(body)
		if err != nil {
			// Log error and continue to next category (don't crash)
			continue
		}

		allItems = append(allItems, items...)
	}

	return allItems, nil
}

// ParseArxivXML parses arXiv Atom XML into FetchedItems.
func ParseArxivXML(data []byte) ([]models.FetchedItem, error) {
	var feed ArxivFeed
	if err := xml.Unmarshal(data, &feed); err != nil {
		return nil, fmt.Errorf("unmarshaling arXiv XML: %w", err)
	}

	var items []models.FetchedItem
	for _, entry := range feed.Entries {
		if entry.ID == "" {
			continue // Skip entries without ID
		}

		publishedAt, err := time.Parse(time.RFC3339, entry.Published)
		if err != nil {
			publishedAt = time.Time{} // Zero value on parse error
		}

		// Find the main URL
		var entryURL string
		for _, link := range entry.Links {
			if link.Rel == "alternate" || link.Type == "text/html" {
				entryURL = link.Href
				break
			}
		}
		if entryURL == "" && len(entry.Links) > 0 {
			entryURL = entry.Links[0].Href
		}

		rawData, _ := json.Marshal(entry)

		items = append(items, models.FetchedItem{
			SourceType:  "arxiv",
			SourceID:    entry.ID,
			Title:       entry.Title,
			Summary:     entry.Summary,
			URL:         entryURL,
			PublishedAt: publishedAt,
			RawData:     string(rawData),
		})
	}

	return items, nil
}

// --- GitHub ---

// GitHubRelease represents a GitHub release from the REST API.
type GitHubRelease struct {
	ID          int64  `json:"id"`
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Body        string `json:"body"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
}

// GitHubFetcher fetches release notes from GitHub repositories.
type GitHubFetcher struct {
	BaseURL    string
	Repos      []string // "owner/repo" format
	HTTPClient *http.Client
}

// NewGitHubFetcher creates a new GitHubFetcher.
func NewGitHubFetcher(baseURL string, repos []string, client *http.Client) *GitHubFetcher {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &GitHubFetcher{
		BaseURL:    baseURL,
		Repos:      repos,
		HTTPClient: client,
	}
}

// Fetch retrieves the latest releases from configured GitHub repositories.
func (f *GitHubFetcher) Fetch(ctx context.Context) ([]models.FetchedItem, error) {
	var allItems []models.FetchedItem

	for _, repo := range f.Repos {
		url := fmt.Sprintf("%s/repos/%s/releases?per_page=5", f.BaseURL, repo)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("creating request for repo %s: %w", repo, err)
		}

		resp, err := f.HTTPClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetching GitHub repo %s: %w", repo, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("GitHub API returned status %d for repo %s", resp.StatusCode, repo)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("reading response body: %w", err)
		}

		items, err := ParseGitHubJSON(body, repo)
		if err != nil {
			continue // Log and skip
		}

		allItems = append(allItems, items...)
	}

	return allItems, nil
}

// ParseGitHubJSON parses GitHub releases JSON into FetchedItems.
func ParseGitHubJSON(data []byte, repo string) ([]models.FetchedItem, error) {
	var releases []GitHubRelease
	if err := json.Unmarshal(data, &releases); err != nil {
		return nil, fmt.Errorf("unmarshaling GitHub JSON: %w", err)
	}

	var items []models.FetchedItem
	for _, rel := range releases {
		if rel.TagName == "" {
			continue
		}

		publishedAt, err := time.Parse(time.RFC3339, rel.PublishedAt)
		if err != nil {
			publishedAt = time.Time{}
		}

		sourceID := fmt.Sprintf("github:%s:%s", repo, rel.TagName)
		rawData, _ := json.Marshal(rel)

		title := rel.Name
		if title == "" {
			title = rel.TagName
		}

		items = append(items, models.FetchedItem{
			SourceType:  "github",
			SourceID:    sourceID,
			Title:       title,
			Summary:     rel.Body,
			URL:         rel.HTMLURL,
			PublishedAt: publishedAt,
			RawData:     string(rawData),
		})
	}

	return items, nil
}

// --- RSS ---

// RSSFetcher fetches entries from RSS/Atom feeds using gofeed.
type RSSFetcher struct {
	FeedURLs   []string
	HTTPClient *http.Client
}

// NewRSSFetcher creates a new RSSFetcher.
func NewRSSFetcher(feedURLs []string, client *http.Client) *RSSFetcher {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &RSSFetcher{
		FeedURLs:   feedURLs,
		HTTPClient: client,
	}
}

// Fetch retrieves the latest entries from configured RSS feeds.
func (f *RSSFetcher) Fetch(ctx context.Context) ([]models.FetchedItem, error) {
	parser := gofeed.NewParser()
	parser.Client = f.HTTPClient

	var allItems []models.FetchedItem

	for _, feedURL := range f.FeedURLs {
		feed, err := parser.ParseURLWithContext(feedURL, ctx)
		if err != nil {
			continue // Log and skip problematic feeds
		}

		items := ParseRSSFeed(feed, feedURL)
		allItems = append(allItems, items...)
	}

	return allItems, nil
}

// ParseRSSFeed converts a gofeed.Feed into FetchedItems.
func ParseRSSFeed(feed *gofeed.Feed, feedURL string) []models.FetchedItem {
	if feed == nil {
		return nil
	}

	var items []models.FetchedItem
	for _, item := range feed.Items {
		if item == nil {
			continue
		}

		var publishedAt time.Time
		if item.PublishedParsed != nil {
			publishedAt = *item.PublishedParsed
		} else if item.UpdatedParsed != nil {
			publishedAt = *item.UpdatedParsed
		}

		sourceID := item.GUID
		if sourceID == "" {
			sourceID = item.Link
		}
		if sourceID == "" {
			continue // Skip items without any identifier
		}

		rawData, _ := json.Marshal(item)

		items = append(items, models.FetchedItem{
			SourceType:  "rss",
			SourceID:    sourceID,
			Title:       item.Title,
			Summary:     item.Description,
			URL:         item.Link,
			PublishedAt: publishedAt,
			RawData:     string(rawData),
		})
	}

	return items
}
