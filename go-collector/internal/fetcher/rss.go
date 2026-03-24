package fetcher

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/autotech-writer/go-collector/internal/common"
	"github.com/autotech-writer/go-collector/internal/models"
	"github.com/mmcdole/gofeed"
)

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

		// Flag items from VIP feeds as breaking news
		if IsVIPRSSFeed(feedURL) {
			for i := range items {
				items[i].IsBreakingNews = true
				if items[i].Score < common.BreakingNewsScoreThreshold {
					items[i].Score = common.BreakingNewsScoreThreshold
				}
			}
		}

		allItems = append(allItems, items...)
	}

	return allItems, nil
}

func (f *RSSFetcher) Name() string {
	return "RSS"
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

		rawData, err := json.Marshal(item)
		if err != nil {
			rawData = []byte("{}")
		}

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
