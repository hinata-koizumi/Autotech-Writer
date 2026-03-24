package fetcher

import (
	"strings"
	"time"
)

// --- Breaking News Configuration ---

// --- Collection Thresholds ---

const (
	// GitHubMinStars is the default threshold for standard repositories.
	GitHubMinStars     = 1000
	// GitHubTrendingStars is the threshold for recent repositories (trending).
	GitHubTrendingStars = 500
	// GitHubTrendingDays is the period to consider a repository as "recent".
	GitHubTrendingDays  = 30
	// HuggingFaceBaseScore is the default score for HF Daily Papers.
	HuggingFaceBaseScore = 30
	// XImpressionThreshold is the default minimum view count for X posts.
	XImpressionThreshold = 10000
)

// BreakingNewsScoreThreshold is the minimum score for an item to be flagged as breaking news.
// Removed: moved to internal/common

// VIPGitHubRepos are repositories whose releases are always treated as breaking news.
var VIPGitHubRepos = map[string]bool{
	"openai/openai-python":            true,
	"anthropics/anthropic-sdk-python":  true,
	"langchain-ai/langchain":           true,
	"vllm-project/vllm":               true,
}

// VIPRSSFeeds are RSS feed URLs whose entries are always treated as breaking news.
var VIPRSSFeeds = map[string]bool{
	"https://openai.com/blog/rss.xml":       true,
	"https://blog.google/technology/ai/rss":  true,
	"https://ai.meta.com/blog/feed.xml":      true,
}

// TargetXUsernames is the list of X handles to monitor for high-impression posts.
var TargetXUsernames = []string{
	"OpenAI",
	"AnthropicAI",
	"GoogleDeepMind",
	"MetaAI",
	"ylecun",
	"karpathy",
}

// --- Polling Schedule Configuration ---

const (
	// VIP sources are polled at high frequency.
	VIPPollInterval    = 3 * time.Minute
	// Normal sources are polled at standard frequency.
	NormalPollInterval = 30 * time.Minute
	// ArXiv high-frequency interval during peak hours (JST 09:00-11:00).
	ArxivPeakInterval  = 5 * time.Minute
	// ArXiv low-frequency interval outside peak hours.
	ArxivOffPeakInterval = 1 * time.Hour
)

// IsVIPGitHubRepo returns true if the given "owner/repo" is a VIP source.
func IsVIPGitHubRepo(repo string) bool {
	return VIPGitHubRepos[strings.ToLower(repo)]
}

// IsVIPRSSFeed returns true if the given feed URL is a VIP source.
func IsVIPRSSFeed(feedURL string) bool {
	return VIPRSSFeeds[feedURL]
}

// --- GitHub Tiers ---
// Removed: moved to internal/common

// --- ArXiv Tiers ---
// Removed: moved to internal/common

// --- Compliance (NG Keywords) ---
// Removed: moved to internal/common

// --- Helpers ---
// Removed: moved to internal/common
