package pipeline

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"events/internal/model"
	"events/internal/repository"
)

// Pipeline implements an async bounded-buffer ingestion pipeline.
// HTTP handlers submit events into a channel; a pool of workers drain the
// channel and batch-insert into ClickHouse. This decouples request latency
// from database write latency.
type Pipeline struct {
	eventCh       chan *model.Event
	repo          repository.EventRepository
	batchSize     int
	flushInterval time.Duration
	workers       int
	wg            sync.WaitGroup
	logger        *slog.Logger

	submitted atomic.Int64
	processed atomic.Int64
	dropped   atomic.Int64
}

func New(
	repo repository.EventRepository,
	bufferSize, workers, batchSize int,
	flushInterval time.Duration,
	logger *slog.Logger,
) *Pipeline {
	return &Pipeline{
		eventCh:       make(chan *model.Event, bufferSize),
		repo:          repo,
		batchSize:     batchSize,
		flushInterval: flushInterval,
		workers:       workers,
		logger:        logger,
	}
}

// Submit enqueues an event for async processing. Returns false if the buffer
// is full (backpressure signal).
func (p *Pipeline) Submit(event *model.Event) bool {
	select {
	case p.eventCh <- event:
		p.submitted.Add(1)
		return true
	default:
		return false
	}
}

func (p *Pipeline) Start() {
	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}
	p.logger.Info("pipeline started",
		"workers", p.workers,
		"buffer_size", cap(p.eventCh),
		"batch_size", p.batchSize,
		"flush_interval", p.flushInterval,
	)
}

// Stop closes the channel and waits for all workers to drain and flush.
func (p *Pipeline) Stop() {
	close(p.eventCh)
	p.wg.Wait()
	p.logger.Info("pipeline stopped",
		"total_submitted", p.submitted.Load(),
		"total_processed", p.processed.Load(),
		"total_dropped", p.dropped.Load(),
	)
}

// Stats returns pipeline counters for observability.
func (p *Pipeline) Stats() (submitted, processed, dropped int64) {
	return p.submitted.Load(), p.processed.Load(), p.dropped.Load()
}

func (p *Pipeline) QueueLen() int {
	return len(p.eventCh)
}

func (p *Pipeline) worker(id int) {
	defer p.wg.Done()

	batch := make([]*model.Event, 0, p.batchSize)
	ticker := time.NewTicker(p.flushInterval)
	defer ticker.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}

		size := len(batch)
		var lastErr error
		for attempt := 0; attempt < 3; attempt++ {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			err := p.repo.InsertBatch(ctx, batch)
			cancel()

			if err == nil {
				p.processed.Add(int64(size))
				p.logger.Debug("batch flushed", "worker", id, "size", size)
				batch = batch[:0]
				return
			}

			lastErr = err
			p.logger.Warn("batch insert attempt failed",
				"worker", id, "attempt", attempt+1, "size", size, "error", err)

			if attempt < 2 {
				time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
			}
		}

		p.dropped.Add(int64(size))
		p.logger.Error("batch dropped after retries",
			"worker", id, "size", size, "error", lastErr)
		batch = batch[:0]
	}

	for {
		select {
		case event, ok := <-p.eventCh:
			if !ok {
				flush()
				return
			}
			batch = append(batch, event)
			if len(batch) >= p.batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}
