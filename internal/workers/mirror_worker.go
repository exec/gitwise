package workers

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/gitwise-io/gitwise/internal/models"
	"github.com/gitwise-io/gitwise/internal/services/mirror"
)

// MirrorWorker periodically reaps stuck runs and dispatches due mirror syncs.
type MirrorWorker struct {
	svc      *mirror.Service
	interval time.Duration
	stopCh   chan struct{}
	stopOnce sync.Once
}

// NewMirrorWorker creates a worker that syncs mirrors at the given interval.
// If interval is <= 0 it defaults to 60 seconds.
func NewMirrorWorker(svc *mirror.Service, interval time.Duration) *MirrorWorker {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	return &MirrorWorker{
		svc:      svc,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// Start begins the background mirror sync loop.
func (w *MirrorWorker) Start() {
	go func() {
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()
		w.tick() // run once immediately so syncs don't wait for the first interval
		for {
			select {
			case <-ticker.C:
				w.tick()
			case <-w.stopCh:
				return
			}
		}
	}()
}

// Stop signals the worker to shut down. Safe to call multiple times.
func (w *MirrorWorker) Stop() { w.stopOnce.Do(func() { close(w.stopCh) }) }

func (w *MirrorWorker) tick() {
	// Bound each tick so a wedged DB doesn't block the next one indefinitely.
	// RunDue blocks on its own WaitGroup, so ticks cannot overlap; no mutex needed.
	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Second)
	defer cancel()

	if reaped, err := w.svc.ReapStuck(ctx); err != nil {
		slog.Error("mirror worker: reap stuck failed", "error", err)
	} else if reaped > 0 {
		slog.Warn("mirror worker: reset abandoned runs", "count", reaped)
	}

	for _, r := range w.svc.RunDue(ctx) {
		attrs := []any{
			"repo_id", r.RepoID,
			"run_id", r.RunID,
			"status", r.Status,
			"refs_changed", r.RefsChanged,
			"duration_ms", r.Duration.Milliseconds(),
			"trigger", "scheduled",
		}
		switch {
		case r.Status == models.MirrorFailed:
			slog.Warn("mirror sync failed", append(attrs, "error", r.Error)...)
		case r.RefsChanged > 0:
			slog.Info("mirror sync", attrs...)
		default:
			// No-op sync (up to date). Keep debug-level so idle instances stay quiet.
			slog.Debug("mirror sync (no-op)", attrs...)
		}
	}
}
