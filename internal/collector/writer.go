package collector

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/trencetech/bayse-orderbook-snapshot/internal/models"
	"github.com/trencetech/bayse-orderbook-snapshot/internal/repository"
)

type BatchWriter struct {
	repo          *repository.SnapshotRepository
	logger        *zap.Logger
	ch            <-chan models.OrderbookSnapshot
	batchSize     int
	flushInterval time.Duration
}

func NewBatchWriter(repo *repository.SnapshotRepository, logger *zap.Logger, ch <-chan models.OrderbookSnapshot) *BatchWriter {
	return &BatchWriter{
		repo:          repo,
		logger:        logger,
		ch:            ch,
		batchSize:     100,
		flushInterval: time.Second,
	}
}

func (w *BatchWriter) Run(ctx context.Context) {
	w.logger.Info("starting batch writer")

	batch := make([]models.OrderbookSnapshot, 0, w.batchSize)
	ticker := time.NewTicker(w.flushInterval)
	defer ticker.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}

		// Use a background context with timeout so the final flush succeeds
		// even if the parent context is cancelled.
		insertCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()

		if err := w.repo.BulkInsert(insertCtx, batch); err != nil {
			w.logger.Error("failed to bulk insert snapshots", zap.Error(err), zap.Int("count", len(batch)))
		} else {
			w.logger.Debug("flushed snapshots", zap.Int("count", len(batch)))
		}

		batch = batch[:0]
	}

	for {
		select {
		case snapshot, ok := <-w.ch:
			if !ok {
				flush()
				w.logger.Info("batch writer stopped")
				return
			}
			batch = append(batch, snapshot)
			if len(batch) >= w.batchSize {
				flush()
			}

		case <-ticker.C:
			flush()
		}
	}
}
