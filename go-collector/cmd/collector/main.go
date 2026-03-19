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
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/autotech?sslmode=disable"
	}
	webhookURL := os.Getenv("WEBHOOK_URL")
	if webhookURL == "" {
		webhookURL = "http://localhost:8000/trigger"
	}

	// 2. Initialize Database connection
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("Database ping failed: %v", err)
	}
	repo := repository.NewRepository(db)

	// 3. Initialize HTTP Client
	client := &http.Client{Timeout: 30 * time.Second}

	// 4. Initialize Fetchers
	arxivCategories := []string{"cs.LG", "cs.SE", "cs.CV", "cs.AI"}
	if cats := os.Getenv("ARXIV_CATEGORIES"); cats != "" {
		arxivCategories = strings.Split(cats, ",")
	}
	arxivFetcher := fetcher.NewArxivFetcher("http://export.arxiv.org", arxivCategories, client)

	githubRepos := []string{"openai/openai-python", "langchain-ai/langchain", "anthropics/anthropic-sdk-python"}
	if repos := os.Getenv("GITHUB_REPOS"); repos != "" {
		githubRepos = strings.Split(repos, ",")
	}
	githubFetcher := fetcher.NewGitHubFetcher("https://api.github.com", githubRepos, client)

	rssFeeds := []string{
		"https://openai.com/blog/rss.xml",
		"https://blog.google/technology/ai/rss",
	}
	if feeds := os.Getenv("RSS_FEEDS"); feeds != "" {
		rssFeeds = strings.Split(feeds, ",")
	}
	rssFetcher := fetcher.NewRSSFetcher(rssFeeds, client)

	fetchers := []fetcher.Fetcher{arxivFetcher, githubFetcher, rssFetcher}

	// 5. Initialize Orchestrator
	orch := orchestrator.NewOrchestrator(fetchers, repo)

	// Determine start mode (oneshot vs daemon)
	if os.Getenv("ONESHOT") == "true" {
		runCollectionAndTrigger(orch, webhookURL)
		log.Println("Oneshot collection finished.")
		return
	}

	// 6. Run continuously (Daemon Mode)
	intervalStr := os.Getenv("FETCH_INTERVAL_MINUTES")
	intervalDuration := 15 * time.Minute
	if intervalStr != "" {
		if idx, err := time.ParseDuration(intervalStr + "m"); err == nil {
			intervalDuration = idx
		}
	}

	log.Printf("Starting collector daemon. Interval: %v", intervalDuration)
	ticker := time.NewTicker(intervalDuration)
	defer ticker.Stop()

	// Run once immediately
	runCollectionAndTrigger(orch, webhookURL)

	// Setup signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case <-ticker.C:
			runCollectionAndTrigger(orch, webhookURL)
		case <-sigCh:
			log.Println("Shutting down collector...")
			return
		}
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
		log.Printf("Webhook returned non-200 status: %d", resp.StatusCode)
	} else {
		log.Println("Webhook successful.")
	}
}
