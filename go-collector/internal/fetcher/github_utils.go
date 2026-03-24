package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/autotech-writer/go-collector/internal/models"
)

var (
	prNumRegex = regexp.MustCompile(`(?m)#(\d+)`)
)

// EnrichWithPRContext adds PR body and file diffs to the fetched items.
func (f *GitHubFetcher) EnrichWithPRContext(ctx context.Context, items []models.FetchedItem, token string) {
	for i := range items {
		if items[i].SourceType != "github" {
			continue
		}
		parts := strings.Split(items[i].SourceID, ":")
		if len(parts) < 3 {
			continue
		}
		repo := parts[1]

		matches := prNumRegex.FindAllStringSubmatch(items[i].Summary, 3)
		var prContexts []string
		for _, match := range matches {
			prNum := match[1]
			if ctx, err := f.fetchPRDetails(ctx, repo, prNum, token); err == nil {
				prContexts = append(prContexts, ctx...)
			}
		}

		if len(prContexts) > 0 {
			items[i].FullContent = strings.Join(prContexts, "\n\n")
		}
	}
}

func (f *GitHubFetcher) fetchPRDetails(ctx context.Context, repo, prNum, token string) ([]string, error) {
	prBody, _ := f.fetchPRBody(ctx, repo, prNum, token)
	prFiles, _ := f.fetchPRFiles(ctx, repo, prNum, token)

	var prContexts []string
	if prBody != "" {
		prContexts = append(prContexts, prBody)
	}
	prContexts = append(prContexts, prFiles...)

	return prContexts, nil
}

func (f *GitHubFetcher) fetchPRBody(ctx context.Context, repo, prNum, token string) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/pulls/%s", f.BaseURL, repo, prNum)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	f.setHeaders(req, token)

	resp, err := f.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("PR body fetch failed: %d", resp.StatusCode)
	}

	var pull struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&pull); err != nil {
		return "", err
	}

	return fmt.Sprintf("PR #%s: %s\n%s", prNum, pull.Title, pull.Body), nil
}

func (f *GitHubFetcher) fetchPRFiles(ctx context.Context, repo, prNum, token string) ([]string, error) {
	url := fmt.Sprintf("%s/repos/%s/pulls/%s/files", f.BaseURL, repo, prNum)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	f.setHeaders(req, token)

	resp, err := f.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("PR files fetch failed: %d", resp.StatusCode)
	}

	var files []struct {
		Filename string `json:"filename"`
		Patch    string `json:"patch"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return nil, err
	}

	var results []string
	for _, file := range files {
		if isValuableFile(file.Filename) && file.Patch != "" {
			results = append(results, fmt.Sprintf("File diff: %s from PR #%s:\n%s", file.Filename, prNum, file.Patch))
		}
	}
	return results, nil
}

// isValuableFile determines if a file in a PR diff is worth sending to the LLM.
func isValuableFile(filename string) bool {
	lowerName := strings.ToLower(filename)

	// Directories to ignore
	ignoredDirs := []string{
		"test/", "tests/", "vendor/", "node_modules/", ".github/",
		"docs/", "examples/", "benchmarks/",
	}
	for _, dir := range ignoredDirs {
		if strings.Contains(lowerName, dir) {
			return false
		}
	}

	// File names/suffixes to ignore
	ignoredSuffices := []string{
		"go.sum", "go.mod", "poetry.lock", "package-lock.json",
		"yarn.lock", "_test.go", "test_out.txt", "license", "makefile",
	}
	for _, suffix := range ignoredSuffices {
		if strings.HasSuffix(lowerName, suffix) {
			return false
		}
	}

	// Specific prefixes to ignore
	if strings.HasPrefix(lowerName, "test_") || strings.HasPrefix(lowerName, ".") {
		return false
	}

	// Allowed extensions for source code/content
	valuableExts := []string{
		".md", ".go", ".py", ".ts", ".js", ".rs", ".java",
		".c", ".cpp", ".h", ".yaml", ".yml", ".json",
	}
	for _, ext := range valuableExts {
		if strings.HasSuffix(lowerName, ext) {
			return true
		}
	}

	return false
}
