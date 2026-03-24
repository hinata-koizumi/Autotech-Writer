package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/autotech-writer/go-collector/internal/models"
)

// XUserFetcher fetches recent high-impression tweets from specific X accounts.
type XUserFetcher struct {
	baseURL     string
	usernames   []string
	bearerToken string
	client      *http.Client
}

// NewXUserFetcher creates a new XUserFetcher.
func NewXUserFetcher(baseURL string, usernames []string, bearerToken string, client *http.Client) *XUserFetcher {
	return &XUserFetcher{
		baseURL:     baseURL,
		usernames:   usernames,
		bearerToken: bearerToken,
		client:      client,
	}
}

func (f *XUserFetcher) Name() string {
	return "x_user"
}

func (f *XUserFetcher) Fetch(ctx context.Context) ([]models.FetchedItem, error) {
	var allItems []models.FetchedItem

	for _, username := range f.usernames {
		// 1. Resolve username to User ID (ideally cached, but for now fetch every time)
		userID, err := f.resolveUsername(ctx, username)
		if err != nil {
			slog.Error("Failed to resolve X username", "username", username, "error", err)
			continue
		}

		// 2. Fetch tweets for user
		tweets, err := f.fetchUserTweets(ctx, userID, username)
		if err != nil {
			slog.Error("Failed to fetch tweets for X user", "username", username, "error", err)
			continue
		}

		allItems = append(allItems, tweets...)
	}

	return allItems, nil
}

type xUserResponse struct {
	Data struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Username string `json:"username"`
	} `json:"data"`
}

func (f *XUserFetcher) resolveUsername(ctx context.Context, username string) (string, error) {
	u := fmt.Sprintf("%s/2/users/by/username/%s", f.baseURL, username)
	body, err := f.getWithAuth(ctx, u)
	if err != nil {
		return "", err
	}

	var resp xUserResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("unmarshaling user response: %w", err)
	}

	if resp.Data.ID == "" {
		return "", fmt.Errorf("user not found: %s", username)
	}

	return resp.Data.ID, nil
}

type xTweetsResponse struct {
	Data []struct {
		ID            string    `json:"id"`
		Text          string    `json:"text"`
		CreatedAt     time.Time `json:"created_at"`
		PublicMetrics struct {
			RetweetCount    int `json:"retweet_count"`
			ReplyCount      int `json:"reply_count"`
			LikeCount       int `json:"like_count"`
			QuoteCount      int `json:"quote_count"`
			ImpressionCount int `json:"impression_count"`
		} `json:"public_metrics"`
	} `json:"data"`
}

func (f *XUserFetcher) fetchUserTweets(ctx context.Context, userID, username string) ([]models.FetchedItem, error) {
	// Request fields: created_at, public_metrics
	params := url.Values{}
	params.Add("tweet.fields", "created_at,public_metrics")
	params.Add("max_results", "10") // Fetch last 10 tweets

	u := fmt.Sprintf("%s/2/users/%s/tweets?%s", f.baseURL, userID, params.Encode())
	body, err := f.getWithAuth(ctx, u)
	if err != nil {
		return nil, err
	}

	var resp xTweetsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("unmarshaling tweets response: %w", err)
	}

	var items []models.FetchedItem
	for _, t := range resp.Data {
		// Filter by impression count
		if t.PublicMetrics.ImpressionCount < XImpressionThreshold {
			continue
		}

		item := models.FetchedItem{
			SourceType:     "x_post",
			SourceID:       t.ID,
			Title:          fmt.Sprintf("Tweet by @%s", username),
			Summary:        t.Text,
			FullContent:    t.Text,
			URL:            fmt.Sprintf("https://twitter.com/%s/status/%s", username, t.ID),
			PublishedAt:    t.CreatedAt,
			Score:          t.PublicMetrics.ImpressionCount,
			IsBreakingNews: t.PublicMetrics.ImpressionCount >= XImpressionThreshold*5, // Arbitrary breaking news threshold
			Metadata: map[string]string{
				"author_username": username,
				"impressions":     fmt.Sprintf("%d", t.PublicMetrics.ImpressionCount),
				"likes":           fmt.Sprintf("%d", t.PublicMetrics.LikeCount),
			},
			RawData: string(body),
		}
		items = append(items, item)
	}

	return items, nil
}

func (f *XUserFetcher) getWithAuth(ctx context.Context, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", f.bearerToken))

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("X API returned status %d: %s", resp.StatusCode, resp.Status)
	}

	return io.ReadAll(resp.Body)
}
