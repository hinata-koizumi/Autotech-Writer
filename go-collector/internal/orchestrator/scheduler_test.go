package orchestrator

import (
	"testing"
	"time"

	"github.com/autotech-writer/go-collector/internal/common"
)

func TestShouldFetchArxivNow(t *testing.T) {
	jst, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		time time.Time
		want bool
	}{
		{
			name: "JST 09:00 — peak start",
			time: time.Date(2026, 3, 22, 9, 0, 0, 0, jst),
			want: true,
		},
		{
			name: "JST 10:30 — within peak",
			time: time.Date(2026, 3, 22, 10, 30, 0, 0, jst),
			want: true,
		},
		{
			name: "JST 10:59 — just before peak end",
			time: time.Date(2026, 3, 22, 10, 59, 0, 0, jst),
			want: true,
		},
		{
			name: "JST 11:00 — peak end (exclusive)",
			time: time.Date(2026, 3, 22, 11, 0, 0, 0, jst),
			want: false,
		},
		{
			name: "JST 08:59 — before peak",
			time: time.Date(2026, 3, 22, 8, 59, 0, 0, jst),
			want: false,
		},
		{
			name: "JST 15:00 — afternoon",
			time: time.Date(2026, 3, 22, 15, 0, 0, 0, jst),
			want: false,
		},
		{
			name: "UTC 00:00 — which is JST 09:00",
			time: time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC),
			want: true,
		},
		{
			name: "UTC 01:59 — which is JST 10:59",
			time: time.Date(2026, 3, 22, 1, 59, 0, 0, time.UTC),
			want: true,
		},
		{
			name: "UTC 02:00 — which is JST 11:00",
			time: time.Date(2026, 3, 22, 2, 0, 0, 0, time.UTC),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := common.ShouldFetchArxivNow(tt.time)
			if got != tt.want {
				t.Errorf("shouldFetchArxivNow(%v) = %v, want %v", tt.time, got, tt.want)
			}
		})
	}
}

func TestIntervalFor(t *testing.T) {
	jst, _ := time.LoadLocation("Asia/Tokyo")
	cfg := DefaultSchedulerConfig()

	tests := []struct {
		name     string
		priority SourcePriority
		now      time.Time
		want     time.Duration
	}{
		{
			name:     "VIP always uses VIP interval",
			priority: PriorityVIP,
			now:      time.Date(2026, 3, 22, 15, 0, 0, 0, jst),
			want:     cfg.VIPInterval,
		},
		{
			name:     "Normal always uses normal interval",
			priority: PriorityNormal,
			now:      time.Date(2026, 3, 22, 15, 0, 0, 0, jst),
			want:     cfg.NormalInterval,
		},
		{
			name:     "ArXiv during peak uses peak interval",
			priority: PriorityArxiv,
			now:      time.Date(2026, 3, 22, 9, 30, 0, 0, jst),
			want:     cfg.ArxivPeak,
		},
		{
			name:     "ArXiv outside peak uses off-peak interval",
			priority: PriorityArxiv,
			now:      time.Date(2026, 3, 22, 15, 0, 0, 0, jst),
			want:     cfg.ArxivOffPeak,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &ScheduledOrchestrator{
				Config:  cfg,
				NowFunc: func() time.Time { return tt.now },
			}
			got := s.intervalFor(tt.priority)
			if got != tt.want {
				t.Errorf("intervalFor(%v) = %v, want %v", tt.priority, got, tt.want)
			}
		})
	}
}
