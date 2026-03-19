package orchestrator

import (
	"context"
	"log"
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
}

// NewOrchestrator creates a new Orchestrator.
func NewOrchestrator(fetchers []fetcher.Fetcher, repo *repository.Repository) *Orchestrator {
	return &Orchestrator{
		Fetchers:   fetchers,
		Repository: repo,
	}
}

// Result tracks the outcome of a single fetch operation.
type Result struct {
	Source    string
	Items    []models.FetchedItem
	Inserted int
	Skipped  int
	Err      error
}

// Run executes all fetchers concurrently within the given context.
// Individual fetcher failures do not block other fetchers.
func (o *Orchestrator) Run(ctx context.Context) []Result {
	var (
		mu      sync.Mutex
		results []Result
		wg      sync.WaitGroup
	)

	for i, f := range o.Fetchers {
		wg.Add(1)
		go func(idx int, ftch fetcher.Fetcher) {
			defer wg.Done()

			result := Result{Source: "fetcher-" + string(rune('0'+idx))}

			items, err := ftch.Fetch(ctx)
			if err != nil {
				result.Err = err
				mu.Lock()
				results = append(results, result)
				mu.Unlock()
				return
			}

			result.Items = items

			for _, item := range items {
				// Sanitize
				sanitized, err := sanitizer.Sanitize(item)
				if err != nil {
					log.Printf("sanitize error for %s: %v", item.SourceID, err)
					continue
				}

				// Insert with dedup
				inserted, err := o.Repository.InsertItem(sanitized)
				if err != nil {
					log.Printf("insert error for %s: %v", sanitized.SourceID, err)
					continue
				}

				if inserted {
					result.Inserted++
				} else {
					result.Skipped++
				}
			}

			mu.Lock()
			results = append(results, result)
			mu.Unlock()
		}(i, f)
	}

	wg.Wait()
	return results
}
