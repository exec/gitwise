package workers

import (
	"context"
	"log/slog"
	"time"

	"github.com/gitwise-io/gitwise/internal/services/mirror"
)

// MirrorWorker periodically reaps stuck runs and dispatches due mirror syncs.
type MirrorWorker struct {
	svc      *mirror.Service
	interval time.Duration
	stopCh   chan struct{}
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

// Stop signals the worker to shut down.
func (w *MirrorWorker) Stop() { close(w.stopCh) }

func (w *MirrorWorker) tick() {
	// Bound each tick so a wedged DB doesn't block the next one indefinitely.
	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Second)
	defer cancel()

	if reaped, err := w.svc.ReapStuck(ctx); err != nil {
		slog.Error("mirror worker: reap stuck failed", "error", err)
	} else if reaped > 0 {
		slog.Warn("mirror worker: reset abandoned runs", "count", reaped)
	}

	for _, r := range w.svc.RunDue(ctx) {
		slog.Info("mirror sync",
			"run_id", r.RunID,
			"status", r.Status,
			"refs_changed", r.RefsChanged,
			"duration_ms", r.Duration.Milliseconds(),
			"error", r.Error,
		)
	}
}
