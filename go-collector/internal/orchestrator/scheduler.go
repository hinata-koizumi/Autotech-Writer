package orchestrator

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/autotech-writer/go-collector/internal/common"
	"github.com/autotech-writer/go-collector/internal/fetcher"
)

// SourcePriority represents the polling priority of a fetcher.
type SourcePriority int

const (
	PriorityVIP    SourcePriority = iota // High-frequency polling (e.g. 3 min)
	PriorityNormal                       // Standard polling (e.g. 30 min)
	PriorityArxiv                        // JST-aware adaptive polling
)

// ScheduledFetcher wraps a Fetcher with its polling priority.
type ScheduledFetcher struct {
	Fetcher  fetcher.Fetcher
	Priority SourcePriority
}

// SchedulerConfig holds the timing parameters for the scheduled orchestrator.
type SchedulerConfig struct {
	VIPInterval    time.Duration
	NormalInterval time.Duration
	ArxivPeak      time.Duration
	ArxivOffPeak   time.Duration
}

// DefaultSchedulerConfig returns the default scheduling configuration.
func DefaultSchedulerConfig() SchedulerConfig {
	return SchedulerConfig{
		VIPInterval:    fetcher.VIPPollInterval,
		NormalInterval: fetcher.NormalPollInterval,
		ArxivPeak:      fetcher.ArxivPeakInterval,
		ArxivOffPeak:   fetcher.ArxivOffPeakInterval,
	}
}

// ScheduledOrchestrator manages fetchers with priority-based polling intervals.
type ScheduledOrchestrator struct {
	Orchestrator *Orchestrator
	Fetchers     []ScheduledFetcher
	Config       SchedulerConfig
	// NowFunc allows overriding the current time for testing.
	NowFunc func() time.Time
}

// NewScheduledOrchestrator creates a new ScheduledOrchestrator.
func NewScheduledOrchestrator(orch *Orchestrator, scheduled []ScheduledFetcher, cfg SchedulerConfig) *ScheduledOrchestrator {
	return &ScheduledOrchestrator{
		Orchestrator: orch,
		Fetchers:     scheduled,
		Config:       cfg,
		NowFunc:      time.Now,
	}
}

// (Removed shouldFetchArxivNow, replaced by common.ShouldFetchArxivNow)

// RunDaemon starts the scheduled polling loop. It dispatches fetcher jobs to the
// existing worker pool at intervals determined by each fetcher's priority.
// The caller should cancel ctx to stop the daemon.
func (s *ScheduledOrchestrator) RunDaemon(ctx context.Context, onCycleComplete func([]Result)) {
	// Group fetchers by priority
	groups := map[SourcePriority][]fetcher.Fetcher{}
	for _, sf := range s.Fetchers {
		groups[sf.Priority] = append(groups[sf.Priority], sf.Fetcher)
	}

	var wg sync.WaitGroup

	// Launch a goroutine for each priority group
	for priority, fetchers := range groups {
		wg.Add(1)
		go func(p SourcePriority, fs []fetcher.Fetcher) {
			defer wg.Done()
			s.runPriorityGroup(ctx, p, fs, onCycleComplete)
		}(priority, fetchers)
	}

	wg.Wait()
}

func (s *ScheduledOrchestrator) runPriorityGroup(
	ctx context.Context,
	priority SourcePriority,
	fetchers []fetcher.Fetcher,
	onCycleComplete func([]Result),
) {
	// Determine initial interval
	interval := s.intervalFor(priority)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run once immediately
	s.dispatchIfAllowed(ctx, priority, fetchers, onCycleComplete)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// For arXiv, dynamically adjust interval based on JST time
			if priority == PriorityArxiv {
				newInterval := s.intervalFor(priority)
				if newInterval != interval {
					interval = newInterval
					ticker.Reset(interval)
					slog.Info("ArXiv polling interval adjusted", "interval", interval)
				}
			}
			s.dispatchIfAllowed(ctx, priority, fetchers, onCycleComplete)
		}
	}
}

// dispatchIfAllowed checks schedule constraints before running fetchers.
func (s *ScheduledOrchestrator) dispatchIfAllowed(
	ctx context.Context,
	priority SourcePriority,
	fetchers []fetcher.Fetcher,
	onCycleComplete func([]Result),
) {
	now := s.NowFunc()

	// For arXiv outside peak hours, skip the fetch entirely
	if priority == PriorityArxiv && !common.ShouldFetchArxivNow(now) {
		slog.Debug("Skipping arXiv fetch outside JST peak hours (09:00-11:00)")
		return
	}

	// Create a temporary orchestrator with only the fetchers for this group
	tmpOrch := s.Orchestrator.WithFetchers(fetchers)

	fetchCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	results := tmpOrch.Run(fetchCtx)

	for _, res := range results {
		if res.Err != nil {
			slog.Error("Scheduled fetch error", "source", res.Source, "priority", priority, "error", res.Err)
		} else {
			slog.Info("Scheduled fetch complete", "source", res.Source, "priority", priority, "inserted", res.Inserted, "skipped", res.Skipped)
		}
	}

	if onCycleComplete != nil {
		onCycleComplete(results)
	}
}

// intervalFor returns the polling interval for a given priority, taking into
// account the current JST time for arXiv sources.
func (s *ScheduledOrchestrator) intervalFor(priority SourcePriority) time.Duration {
	switch priority {
	case PriorityVIP:
		return s.Config.VIPInterval
	case PriorityArxiv:
		if common.ShouldFetchArxivNow(s.NowFunc()) {
			return s.Config.ArxivPeak
		}
		return s.Config.ArxivOffPeak
	default:
		return s.Config.NormalInterval
	}
}
