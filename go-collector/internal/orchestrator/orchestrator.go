package orchestrator

import (
	"context"
	"log/slog"
	"sync"

	"github.com/autotech-writer/go-collector/internal/fetcher"
	"github.com/autotech-writer/go-collector/internal/models"
	"github.com/autotech-writer/go-collector/internal/repository"
	"github.com/autotech-writer/go-collector/internal/sanitizer"
)

// Orchestrator manages concurrent fetching from multiple data sources.
type Orchestrator struct {
	Fetchers   []fetcher.Fetcher
	Repository *repository.Repository
	MaxWorkers int
}

// NewOrchestrator creates a new Orchestrator with a default worker limit.
func NewOrchestrator(fetchers []fetcher.Fetcher, repo *repository.Repository) *Orchestrator {
	return &Orchestrator{
		Fetchers:   fetchers,
		Repository: repo,
		MaxWorkers: 5,
	}
}

// Result tracks the outcome of a single fetch operation.
type Result struct {
	Source   string
	Items    []models.FetchedItem
	Inserted int
	Skipped  int
	Err      error
}

// Run executes all fetchers using a worker pool within the given context.
func (o *Orchestrator) Run(ctx context.Context) []Result {
	o.Prune(ctx, 30, 180)

	totalFetchers := len(o.Fetchers)
	results := make([]Result, 0, totalFetchers)
	resultsChan := make(chan Result, totalFetchers)
	fetcherChan := make(chan fetcher.Fetcher, totalFetchers)

	var wg sync.WaitGroup

	// Start workers
	numWorkers := o.MaxWorkers
	if totalFetchers < numWorkers {
		numWorkers = totalFetchers
	}

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for f := range fetcherChan {
				resultsChan <- o.fetchAndProcess(ctx, f)
			}
		}()
	}

	// Feed fetchers
	for _, f := range o.Fetchers {
		fetcherChan <- f
	}
	close(fetcherChan)

	// Collect results
	wg.Wait()
	close(resultsChan)

	for res := range resultsChan {
		results = append(results, res)
	}

	return results
}

func (o *Orchestrator) fetchAndProcess(ctx context.Context, f fetcher.Fetcher) Result {
	slog.Info("Starting fetch for source", "source", f.Name())
	res := Result{Source: f.Name()}

	items, err := f.Fetch(ctx)
	if err != nil {
		slog.Error("Fetch failed", "source", f.Name(), "error", err)
		res.Err = err
		_ = o.Repository.RecordStats(f.Name(), 0, 0, 1)
		return res
	}

	res.Items = items
	res.Inserted, res.Skipped, err = o.processItems(ctx, f.Name(), items)
	if err != nil {
		res.Err = err
	}

	return res
}

type itemProcessResult struct {
	inserted int
	skipped  int
	errors   int
}

func (o *Orchestrator) processItems(ctx context.Context, sourceName string, items []models.FetchedItem) (int, int, error) {
	var totalInserted, totalSkipped, totalErrors int

	for _, item := range items {
		select {
		case <-ctx.Done():
			return totalInserted, totalSkipped, ctx.Err()
		default:
		}

		res := o.processItem(ctx, item)
		totalInserted += res.inserted
		totalSkipped += res.skipped
		totalErrors += res.errors
	}

	// Record cycle stats
	if err := o.Repository.RecordStats(sourceName, len(items), totalSkipped, totalErrors); err != nil {
		slog.Error("Failed to record stats", "source", sourceName, "error", err)
	}

	slog.Info("Finished processing source", "source", sourceName, "inserted", totalInserted, "skipped", totalSkipped, "errors", totalErrors)
	return totalInserted, totalSkipped, nil
}

func (o *Orchestrator) processItem(ctx context.Context, item models.FetchedItem) itemProcessResult {
	// 1. Existence check
	exists, err := o.Repository.ItemExists(item.SourceID)
	if err != nil {
		slog.Error("Existence check error", "id", item.SourceID, "error", err)
		return itemProcessResult{errors: 1}
	}
	if exists {
		return itemProcessResult{skipped: 1}
	}

	// 2. Sanitize
	sanitized, err := sanitizer.Sanitize(item)
	if err != nil {
		slog.Error("Sanitize error", "id", item.SourceID, "error", err)
		return itemProcessResult{errors: 1}
	}

	// 3. Insert
	inserted, err := o.Repository.InsertItem(sanitized)
	if err != nil {
		slog.Error("Insert error", "id", sanitized.SourceID, "error", err)
		return itemProcessResult{errors: 1}
	}

	if inserted {
		return itemProcessResult{inserted: 1}
	}
	// This usually means score was 0 but item was marked as seen (skipped further processing)
	return itemProcessResult{skipped: 1}
}

// WithFetchers returns a copy of the Orchestrator with a different set of fetchers.
func (o *Orchestrator) WithFetchers(fetchers []fetcher.Fetcher) *Orchestrator {
	return &Orchestrator{
		Fetchers:   fetchers,
		Repository: o.Repository,
		MaxWorkers: o.MaxWorkers,
	}
}

// Prune removes old articles from processing and old history from seen_articles.
func (o *Orchestrator) Prune(ctx context.Context, processingDays, seenDays int) {
	// 1. Prune processing table (30 days by default)
	deletedProc, err := o.Repository.PruneOldArticles(processingDays)
	if err != nil {
		slog.Error("Failed to prune articles", "error", err)
	} else if deletedProc > 0 {
		slog.Info("Pruned old articles from processing", "count", deletedProc)
	}

	// 2. Prune seen_articles history (180 days by default)
	deletedSeen, err := o.Repository.PruneSeenArticles(seenDays)
	if err != nil {
		slog.Error("Failed to prune seen articles", "error", err)
	} else if deletedSeen > 0 {
		slog.Info("Pruned old seen articles history", "count", deletedSeen)
	}
}
