package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/autotech-writer/go-collector/internal/fetcher"
	"github.com/autotech-writer/go-collector/internal/orchestrator"
	"github.com/autotech-writer/go-collector/internal/repository"
	_ "github.com/lib/pq"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("Starting Autotech Writer Collector...")

	// 1. Determine environment configuration
	dbURL := getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/autotech?sslmode=disable")
	webhookURL := getEnv("WEBHOOK_URL", "http://localhost:8000/trigger")
	xBearerToken := os.Getenv("X_BEARER_TOKEN")

	// 2. Initialize Repository
	repo, db := initRepo(dbURL)
	defer db.Close()

	// 3. Initialize Fetchers
	client := &http.Client{Timeout: 30 * time.Second}
	arxivCategories := getArxivCategories()
	githubRepos := getGitHubRepos()
	rssFeeds := getRSSFeeds()

	fetchers := initFetchers(client, arxivCategories, githubRepos, rssFeeds, xBearerToken)

	// 4. Initialize Orchestrator
	orch := orchestrator.NewOrchestrator(fetchers, repo)

	// 5. Run in One-shot mode if requested
	if os.Getenv("ONESHOT") == "true" {
		runCollectionAndTrigger(orch, webhookURL)
		log.Println("Oneshot collection finished.")
		return
	}

	// 6. Run continuously (Daemon Mode) with priority-based scheduling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	setupSignalHandler(cancel)

	scheduled := initScheduledFetchers(client, githubRepos, rssFeeds, arxivCategories, xBearerToken)
	cfg := orchestrator.DefaultSchedulerConfig()
	schedOrch := orchestrator.NewScheduledOrchestrator(orch, scheduled, cfg)

	log.Printf("Starting scheduled collector daemon (VIP: %v, Normal: %v, ArXiv peak: %v, off-peak: %v)",
		cfg.VIPInterval, cfg.NormalInterval, cfg.ArxivPeak, cfg.ArxivOffPeak)

	schedOrch.RunDaemon(ctx, func(results []orchestrator.Result) {
		handleResults(results, webhookURL)
	})
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func getArxivCategories() []string {
	cats := os.Getenv("ARXIV_CATEGORIES")
	if cats == "" {
		return []string{"cs.LG", "cs.SE", "cs.CV", "cs.AI", "cs.CL", "cs.CR", "cs.DC"}
	}
	return strings.Split(cats, ",")
}

func getGitHubRepos() []string {
	repos := os.Getenv("GITHUB_REPOS")
	if repos == "" {
		return []string{
			"openai/openai-python", "langchain-ai/langchain", "anthropics/anthropic-sdk-python",
			"vllm-project/vllm", "ollama/ollama", "ultralytics/ultralytics",
			"tiangolo/fastapi", "docker/compose",
		}
	}
	return strings.Split(repos, ",")
}

func getRSSFeeds() []string {
	feeds := os.Getenv("RSS_FEEDS")
	if feeds == "" {
		return []string{
			"https://openai.com/blog/rss.xml",
			"https://blog.google/technology/ai/rss",
			"https://huggingface.co/blog/feed.xml",
			"https://ai.meta.com/blog/feed.xml",
			"https://aws.amazon.com/blogs/machine-learning/feed/",
			"https://blog.cloudflare.com/rss/",
		}
	}
	return strings.Split(feeds, ",")
}

func initRepo(dbURL string) (*repository.Repository, *sql.DB) {
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}

	if err := db.Ping(); err != nil {
		log.Fatalf("Database ping failed: %v", err)
	}
	return repository.NewRepository(db), db
}

func initFetchers(client *http.Client, arxivCats, ghRepos, rssFeeds []string, xToken string) []fetcher.Fetcher {
	var fetchers []fetcher.Fetcher
	fetchers = append(fetchers, fetcher.NewArxivFetcher("http://export.arxiv.org", arxivCats, client))
	fetchers = append(fetchers, fetcher.NewGitHubFetcher("https://api.github.com", ghRepos, client))
	fetchers = append(fetchers, fetcher.NewRSSFetcher(rssFeeds, client))
	fetchers = append(fetchers, fetcher.NewHuggingFaceFetcher("https://huggingface.co", "https://export.arxiv.org", client))

	if xToken != "" {
		fetchers = append(fetchers, fetcher.NewXUserFetcher("https://api.twitter.com", fetcher.TargetXUsernames, xToken, client))
	}
	return fetchers
}

func setupSignalHandler(cancel context.CancelFunc) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("Shutting down collector...")
		cancel()
	}()
}

func handleResults(results []orchestrator.Result, webhookURL string) {
	var totalInserted int
	for _, res := range results {
		if res.Err != nil {
			log.Printf("Error from %s: %v", res.Source, res.Err)
		} else {
			totalInserted += res.Inserted
		}
	}
	if totalInserted > 0 && webhookURL != "" {
		triggerPythonPipeline(webhookURL)
	}
}

// runCollectionAndTrigger runs the orchestrator once and triggers the python LLM service
func runCollectionAndTrigger(orch *orchestrator.Orchestrator, webhookURL string) {
	log.Println("Starting fetch cycle...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	results := orch.Run(ctx)

	var totalInserted int
	for _, res := range results {
		if res.Err != nil {
			log.Printf("Error from %s: %v", res.Source, res.Err)
		} else {
			log.Printf("Source %s: Inserted %d items, Skipped %d items", res.Source, res.Inserted, res.Skipped)
			totalInserted += res.Inserted
		}
	}

	// If items were inserted, trigger the Python service via Webhook
	if totalInserted > 0 && webhookURL != "" {
		triggerPythonPipeline(webhookURL)
	}
}

func triggerPythonPipeline(webhookURL string) {
	log.Printf("Triggering python pipeline at %s", webhookURL)
	payload := map[string]string{"event": "new_articles"}
	body, _ := json.Marshal(payload)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(webhookURL, "application/json", bytes.NewBuffer(body))
	if err != nil {
		log.Printf("Failed to trigger webhook: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("Webhook returned non-200 status: %d. (Python worker might be down or busy)", resp.StatusCode)
	} else {
		log.Println("Webhook successful.")
	}
}

func initScheduledFetchers(client *http.Client, githubRepos []string, rssFeeds []string, arxivCategories []string, xBearerToken string) []orchestrator.ScheduledFetcher {
	// Build scheduled fetcher list with VIP/normal/arXiv classification.
	// - ArXiv: adaptive polling based on JST peak hours (09:00-11:00)
	// - VIP GitHub repos & RSS feeds: high-frequency polling (3 min)
	// - Normal sources: standard polling (30 min)

	// Split GitHub repos into VIP and normal fetchers
	var vipGHRepos, normalGHRepos []string
	for _, r := range githubRepos {
		if fetcher.IsVIPGitHubRepo(r) {
			vipGHRepos = append(vipGHRepos, r)
		} else {
			normalGHRepos = append(normalGHRepos, r)
		}
	}

	// Split RSS feeds into VIP and normal fetchers
	var vipRSSFeeds, normalRSSFeeds []string
	for _, feed := range rssFeeds {
		if fetcher.IsVIPRSSFeed(feed) {
			vipRSSFeeds = append(vipRSSFeeds, feed)
		} else {
			normalRSSFeeds = append(normalRSSFeeds, feed)
		}
	}

	var scheduled []orchestrator.ScheduledFetcher

	// ArXiv — adaptive JST-aware schedule
	scheduled = append(scheduled, orchestrator.ScheduledFetcher{
		Fetcher:  fetcher.NewArxivFetcher("http://export.arxiv.org", arxivCategories, client),
		Priority: orchestrator.PriorityArxiv,
	})

	// VIP GitHub
	if len(vipGHRepos) > 0 {
		scheduled = append(scheduled, orchestrator.ScheduledFetcher{
			Fetcher:  fetcher.NewGitHubFetcher("https://api.github.com", vipGHRepos, client),
			Priority: orchestrator.PriorityVIP,
		})
	}
	// Normal GitHub
	if len(normalGHRepos) > 0 {
		scheduled = append(scheduled, orchestrator.ScheduledFetcher{
			Fetcher:  fetcher.NewGitHubFetcher("https://api.github.com", normalGHRepos, client),
			Priority: orchestrator.PriorityNormal,
		})
	}

	// VIP RSS
	if len(vipRSSFeeds) > 0 {
		scheduled = append(scheduled, orchestrator.ScheduledFetcher{
			Fetcher:  fetcher.NewRSSFetcher(vipRSSFeeds, client),
			Priority: orchestrator.PriorityVIP,
		})
	}
	// Normal RSS
	if len(normalRSSFeeds) > 0 {
		scheduled = append(scheduled, orchestrator.ScheduledFetcher{
			Fetcher:  fetcher.NewRSSFetcher(normalRSSFeeds, client),
			Priority: orchestrator.PriorityNormal,
		})
	}

	scheduled = append(scheduled, orchestrator.ScheduledFetcher{
		Fetcher:  fetcher.NewHuggingFaceFetcher("https://huggingface.co", "https://export.arxiv.org", client),
		Priority: orchestrator.PriorityNormal,
	})

	// X (Target Accounts)
	if xBearerToken != "" {
		xFetcher := fetcher.NewXUserFetcher("https://api.twitter.com", fetcher.TargetXUsernames, xBearerToken, client)
		scheduled = append(scheduled, orchestrator.ScheduledFetcher{
			Fetcher:  xFetcher,
			Priority: orchestrator.PriorityNormal,
		})
	}

	return scheduled
}
