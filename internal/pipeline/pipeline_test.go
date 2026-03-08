package pipeline

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"events/internal/model"
	"events/internal/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockRepo struct {
	mu     sync.Mutex
	events []*model.Event
}

func (m *mockRepo) InsertBatch(_ context.Context, events []*model.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, events...)
	return nil
}

func (m *mockRepo) QueryMetrics(_ context.Context, _ string, _, _ time.Time, _ string) (*repository.MetricsResult, error) {
	return &repository.MetricsResult{}, nil
}

func (m *mockRepo) totalEvents() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.events)
}

func newTestEvent(name string) *model.Event {
	return &model.Event{
		EventID:    "test-id-" + name,
		EventName:  name,
		UserID:     "user-1",
		Timestamp:  time.Now().UTC(),
		Tags:       []string{},
		Metadata:   "{}",
		IngestedAt: time.Now().UTC(),
	}
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestPipeline_SubmitAndFlush(t *testing.T) {
	repo := &mockRepo{}
	p := New(repo, 100, 1, 10, 50*time.Millisecond, quietLogger())
	p.Start()

	for i := 0; i < 25; i++ {
		ok := p.Submit(newTestEvent("event"))
		require.True(t, ok)
	}

	time.Sleep(300 * time.Millisecond)
	p.Stop()

	assert.Equal(t, int64(25), p.processed.Load())
	assert.Equal(t, 25, repo.totalEvents())
}

func TestPipeline_Backpressure(t *testing.T) {
	repo := &mockRepo{}
	// Buffer of 5 with 0 workers — nothing drains
	p := New(repo, 5, 0, 10, time.Hour, quietLogger())

	for i := 0; i < 5; i++ {
		ok := p.Submit(newTestEvent("event"))
		assert.True(t, ok)
	}

	ok := p.Submit(newTestEvent("overflow"))
	assert.False(t, ok, "should signal backpressure when buffer is full")
}

func TestPipeline_BatchSizeTrigger(t *testing.T) {
	repo := &mockRepo{}
	p := New(repo, 100, 1, 5, time.Hour, quietLogger())
	p.Start()

	for i := 0; i < 5; i++ {
		p.Submit(newTestEvent("event"))
	}

	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, 5, repo.totalEvents())
	p.Stop()
}

func TestPipeline_GracefulShutdownFlush(t *testing.T) {
	repo := &mockRepo{}
	p := New(repo, 100, 1, 1000, time.Hour, quietLogger())
	p.Start()

	for i := 0; i < 3; i++ {
		p.Submit(newTestEvent("event"))
	}

	p.Stop()
	assert.Equal(t, 3, repo.totalEvents(), "shutdown must flush in-flight events")
}

func TestPipeline_Stats(t *testing.T) {
	repo := &mockRepo{}
	p := New(repo, 100, 2, 100, 50*time.Millisecond, quietLogger())
	p.Start()

	for i := 0; i < 10; i++ {
		p.Submit(newTestEvent("event"))
	}

	time.Sleep(200 * time.Millisecond)
	p.Stop()

	submitted, processed, dropped := p.Stats()
	assert.Equal(t, int64(10), submitted)
	assert.Equal(t, int64(10), processed)
	assert.Equal(t, int64(0), dropped)
}
