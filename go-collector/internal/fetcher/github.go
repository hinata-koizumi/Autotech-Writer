package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/autotech-writer/go-collector/internal/common"
	"github.com/autotech-writer/go-collector/internal/models"
)

var (
	semverRegex = regexp.MustCompile(`:v?(\d+)\.(\d+)\.0(?:[+-].*)?$`)
)

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
	BaseURL       string
	Repos         []string // "owner/repo" format
	PerPage       int
	StarThreshold int
	HTTPClient    *http.Client
}

// NewGitHubFetcher creates a new GitHubFetcher.
func NewGitHubFetcher(baseURL string, repos []string, client *http.Client) *GitHubFetcher {
	if baseURL == "" {
		baseURL = "https://api.github.com"
	}
	if client == nil {
		client = &http.Client{Timeout: DefaultHTTPTimeout}
	}
	return &GitHubFetcher{
		BaseURL:       baseURL,
		Repos:         repos,
		PerPage:       DefaultGitHubPerPage,
		StarThreshold: GitHubMinStars,
		HTTPClient:    client,
	}
}

// Fetch retrieves the latest releases from configured GitHub repositories.
func (f *GitHubFetcher) Fetch(ctx context.Context) ([]models.FetchedItem, error) {
	ghToken := os.Getenv("GH_TOKEN")
	
	eligibleRepos, repoStars := f.filterReposByStars(ctx, ghToken)
	if len(eligibleRepos) == 0 {
		return nil, nil
	}

	items, err := f.fetchReleases(ctx, eligibleRepos)
	if err != nil {
		return items, err
	}

	filteredItems := f.filterAndEnrichItems(items, repoStars)
	f.EnrichWithPRContext(ctx, filteredItems, ghToken)
	
	return filteredItems, nil
}

func (f *GitHubFetcher) fetchReleases(ctx context.Context, repos []string) ([]models.FetchedItem, error) {
	urls := make([]string, len(repos))
	for i, repo := range repos {
		urls[i] = fmt.Sprintf("%s/repos/%s/releases?per_page=%d", f.BaseURL, repo, f.PerPage)
	}

	return collectFromSources(ctx, f.HTTPClient, repos, urls, ParseGitHubJSON)
}

func (f *GitHubFetcher) filterReposByStars(ctx context.Context, token string) ([]string, map[string]int) {
	type repoResult struct {
		repo      string
		stars     int
		createdAt time.Time
		err       error
	}

	resultsChan := make(chan repoResult, len(f.Repos))
	var wg sync.WaitGroup

	for _, repo := range f.Repos {
		wg.Add(1)
		go func(r string) {
			defer wg.Done()
			stars, createdAt, err := f.getRepoStars(ctx, r, token)
			resultsChan <- repoResult{repo: r, stars: stars, createdAt: createdAt, err: err}
		}(repo)
	}

	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	var filtered []string
	starsMap := make(map[string]int)
	
	for res := range resultsChan {
		if res.err != nil {
			slog.Warn("Failed to get stars for repo", "repo", res.repo, "error", res.err)
			continue
		}

		isTrending := res.stars >= GitHubTrendingStars && time.Since(res.createdAt).Hours() < 24*float64(GitHubTrendingDays)
		if res.stars < f.StarThreshold && !isTrending {
			continue
		}
		filtered = append(filtered, res.repo)
		starsMap[res.repo] = res.stars
	}
	return filtered, starsMap
}

func (f *GitHubFetcher) filterAndEnrichItems(items []models.FetchedItem, repoStars map[string]int) []models.FetchedItem {
	var filtered []models.FetchedItem
	for _, item := range items {
		if !semverRegex.MatchString(item.SourceID) {
			continue
		}

		f.enrichItemWithStarsAndScore(&item, repoStars)
		
		filtered = append(filtered, item)
	}
	return filtered
}

func (f *GitHubFetcher) enrichItemWithStarsAndScore(item *models.FetchedItem, repoStars map[string]int) {
	// Extract repo name from SourceID (github:owner/repo:v1.0.0)
	parts := strings.Split(item.SourceID, ":")
	if len(parts) < 2 {
		return
	}

	repoPath := parts[1]
	if stars, ok := repoStars[repoPath]; ok {
		if item.Metadata == nil {
			item.Metadata = make(map[string]string)
		}
		item.Metadata["stars"] = fmt.Sprintf("%d", stars)
	}
	f.evaluateOwnerScore(item, repoPath)

	// Flag as breaking news if VIP repo or score meets threshold
	if IsVIPGitHubRepo(repoPath) || item.Score >= common.BreakingNewsScoreThreshold {
		item.IsBreakingNews = true
	}
}

func (f *GitHubFetcher) evaluateOwnerScore(item *models.FetchedItem, repoPath string) {
	parts := strings.Split(repoPath, "/")
	if len(parts) < 1 {
		return
	}
	owner := parts[0]
	if score, ok := common.IsRenownedOwner(owner); ok {
		item.Score = score
		item.Metadata["renowned_owner"] = strings.ToLower(owner)
	}
}

func (f *GitHubFetcher) getRepoStars(ctx context.Context, repo string, token string) (int, time.Time, error) {
	url := fmt.Sprintf("%s/repos/%s", f.BaseURL, repo)
	
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("creating repo request for %s: %w", repo, err)
	}
	f.setHeaders(req, token)

	resp, err := f.HTTPClient.Do(req)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("performing repo request for %s: %w", repo, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return 0, time.Time{}, fmt.Errorf("repository %s not found (HTTP 404)", repo)
	}
	if resp.StatusCode != http.StatusOK {
		return 0, time.Time{}, fmt.Errorf("unexpected status from GitHub for %s: %d (%s)", repo, resp.StatusCode, resp.Status)
	}

	var data struct {
		StargazersCount int       `json:"stargazers_count"`
		CreatedAt       time.Time `json:"created_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return 0, time.Time{}, fmt.Errorf("decoding repo response for %s: %w", repo, err)
	}

	return data.StargazersCount, data.CreatedAt, nil
}

func (f *GitHubFetcher) setHeaders(req *http.Request, token string) {
	req.Header.Set("User-Agent", "Autotech-Writer/1.0")
	req.Header.Set("Accept", "application/vnd.github+json")
	if token != "" {
		req.Header.Set("Authorization", "token "+token)
	}
}

func (f *GitHubFetcher) Name() string {
	return "GitHub"
}

// ParseGitHubJSON parses GitHub releases JSON into FetchedItems.
func ParseGitHubJSON(data []byte, source string) ([]models.FetchedItem, error) {
	var releases []GitHubRelease
	if err := json.Unmarshal(data, &releases); err != nil {
		return nil, fmt.Errorf("unmarshaling GitHub JSON: %w", err)
	}

	var items []models.FetchedItem
	for _, rel := range releases {
		if rel.TagName == "" {
			continue
		}

		publishedAt, _ := time.Parse(time.RFC3339, rel.PublishedAt)
		sourceID := fmt.Sprintf("github:%s:%s", source, rel.TagName)
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
			Metadata:    make(map[string]string),
			RawData:     string(rawData),
		})
	}
	return items, nil
}
