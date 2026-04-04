package workers

import (
	"context"
	"log/slog"
	"time"

	"github.com/gitwise-io/gitwise/internal/services/embedding"
)

// EmbeddingWorker periodically backfills missing embeddings across tables.
type EmbeddingWorker struct {
	embeddingSvc *embedding.Service
	interval     time.Duration
	stopCh       chan struct{}
}

// NewEmbeddingWorker creates a worker that backfills embeddings at the given interval.
func NewEmbeddingWorker(embeddingSvc *embedding.Service, interval time.Duration) *EmbeddingWorker {
	return &EmbeddingWorker{
		embeddingSvc: embeddingSvc,
		interval:     interval,
		stopCh:       make(chan struct{}),
	}
}

// Start begins the background backfill loop.
func (w *EmbeddingWorker) Start() {
	go func() {
		// Run once immediately on start
		w.run()

		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				w.run()
			case <-w.stopCh:
				return
			}
		}
	}()
}

// Stop signals the worker to shut down.
func (w *EmbeddingWorker) Stop() {
	close(w.stopCh)
}

// backfillTarget defines a table and its columns to backfill.
type backfillTarget struct {
	table           string
	idColumn        string
	textColumn      string
	embeddingColumn string
}

var targets = []backfillTarget{
	{"issues", "id", "title", "title_embedding"},
	{"issues", "id", "body", "body_embedding"},
	{"pull_requests", "id", "title", "title_embedding"},
	{"pull_requests", "id", "body", "body_embedding"},
	{"commit_metadata", "sha", "message", "message_embedding"},
	{"repositories", "id", "description", "description_embedding"},
}

func (w *EmbeddingWorker) run() {
	if !w.embeddingSvc.IsEnabled() {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	totalProcessed := 0
	for _, t := range targets {
		n, err := w.embeddingSvc.BackfillTable(ctx, t.table, t.idColumn, t.textColumn, t.embeddingColumn, 50)
		if err != nil {
			slog.Warn("embedding backfill failed", "table", t.table, "column", t.embeddingColumn, "error", err)
			continue
		}
		totalProcessed += n
	}

	if totalProcessed > 0 {
		slog.Info("embedding backfill complete", "processed", totalProcessed)
	}
}
