package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"events/internal/dedup"
	"events/internal/dto"
	"events/internal/model"
	"events/internal/pipeline"
	"events/internal/repository"
	"events/internal/service"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type noopRepo struct{}

func (r *noopRepo) InsertBatch(_ context.Context, _ []*model.Event) error {
	return nil
}

func (r *noopRepo) QueryMetrics(_ context.Context, _ string, _, _ time.Time, _ string) (*repository.MetricsResult, error) {
	return &repository.MetricsResult{}, nil
}

func setupEventHandler(t *testing.T) (*EventHandler, *pipeline.Pipeline) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	repo := &noopRepo{}
	pipe := pipeline.New(repo, 1000, 1, 100, time.Second, logger)
	pipe.Start()
	cache := dedup.NewCache(10000)
	svc := service.NewEventService(pipe, cache)
	return NewEventHandler(svc), pipe
}

func TestPostEvent_Valid(t *testing.T) {
	h, pipe := setupEventHandler(t)
	defer pipe.Stop()

	body := dto.EventRequest{
		EventName: "product_view",
		UserID:    "user_123",
		Timestamp: time.Now().Unix(),
		Channel:   "web",
	}
	b, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/events", bytes.NewReader(b))
	w := httptest.NewRecorder()
	h.PostEvent(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)

	var resp dto.EventResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "accepted", resp.Status)
	assert.Len(t, resp.EventID, 32)
}

func TestPostEvent_Duplicate(t *testing.T) {
	h, pipe := setupEventHandler(t)
	defer pipe.Stop()

	body := dto.EventRequest{
		EventName: "purchase",
		UserID:    "user_456",
		Timestamp: time.Now().Unix(),
	}

	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/events", bytes.NewReader(b))
	w := httptest.NewRecorder()
	h.PostEvent(w, req)
	assert.Equal(t, http.StatusAccepted, w.Code)

	b2, _ := json.Marshal(body)
	req2 := httptest.NewRequest(http.MethodPost, "/events", bytes.NewReader(b2))
	w2 := httptest.NewRecorder()
	h.PostEvent(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)

	var resp dto.EventResponse
	json.Unmarshal(w2.Body.Bytes(), &resp)
	assert.Equal(t, "duplicate", resp.Status)
}

func TestPostEvent_InvalidJSON(t *testing.T) {
	h, pipe := setupEventHandler(t)
	defer pipe.Stop()

	req := httptest.NewRequest(http.MethodPost, "/events", bytes.NewBufferString("{invalid"))
	w := httptest.NewRecorder()
	h.PostEvent(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp dto.ErrorResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, "INVALID_JSON", resp.Code)
}

func TestPostEvent_MissingFields(t *testing.T) {
	h, pipe := setupEventHandler(t)
	defer pipe.Stop()

	body := dto.EventRequest{}
	b, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/events", bytes.NewReader(b))
	w := httptest.NewRecorder()
	h.PostEvent(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp dto.ErrorResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, "VALIDATION_ERROR", resp.Code)
	assert.NotEmpty(t, resp.Details)
}

func TestPostBulkEvents_Valid(t *testing.T) {
	h, pipe := setupEventHandler(t)
	defer pipe.Stop()

	body := dto.BulkEventRequest{
		Events: []dto.EventRequest{
			{EventName: "view", UserID: "u1", Timestamp: time.Now().Unix()},
			{EventName: "click", UserID: "u2", Timestamp: time.Now().Unix()},
		},
	}
	b, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/events/bulk", bytes.NewReader(b))
	w := httptest.NewRecorder()
	h.PostBulkEvents(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)

	var resp dto.BulkEventResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, 2, resp.Accepted)
	assert.Equal(t, 0, resp.Rejected)
}

func TestPostBulkEvents_MixedValidity(t *testing.T) {
	h, pipe := setupEventHandler(t)
	defer pipe.Stop()

	body := dto.BulkEventRequest{
		Events: []dto.EventRequest{
			{EventName: "view", UserID: "u1", Timestamp: time.Now().Unix()},
			{EventName: "", UserID: "", Timestamp: 0},
		},
	}
	b, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/events/bulk", bytes.NewReader(b))
	w := httptest.NewRecorder()
	h.PostBulkEvents(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)

	var resp dto.BulkEventResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, 1, resp.Accepted)
	assert.Equal(t, 1, resp.Rejected)
}

func TestPostEvent_BackpressureThenRetry(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	repo := &noopRepo{}
	pipe := pipeline.New(repo, 1, 0, 10, time.Hour, logger)

	cache := dedup.NewCache(10000)
	svc := service.NewEventService(pipe, cache)
	h := NewEventHandler(svc)

	filler := dto.EventRequest{
		EventName: "filler",
		UserID:    "user_fill",
		Timestamp: time.Now().Unix(),
	}
	b, _ := json.Marshal(filler)
	req := httptest.NewRequest(http.MethodPost, "/events", bytes.NewReader(b))
	w := httptest.NewRecorder()
	h.PostEvent(w, req)
	assert.Equal(t, http.StatusAccepted, w.Code)

	target := dto.EventRequest{
		EventName: "retry_event",
		UserID:    "user_bp",
		Timestamp: time.Now().Unix(),
	}
	b2, _ := json.Marshal(target)
	req2 := httptest.NewRequest(http.MethodPost, "/events", bytes.NewReader(b2))
	w2 := httptest.NewRecorder()
	h.PostEvent(w2, req2)
	assert.Equal(t, http.StatusServiceUnavailable, w2.Code)

	pipe2 := pipeline.New(repo, 100, 1, 10, time.Second, logger)
	pipe2.Start()
	defer pipe2.Stop()

	svc2 := service.NewEventService(pipe2, cache)
	h2 := NewEventHandler(svc2)

	b3, _ := json.Marshal(target)
	req3 := httptest.NewRequest(http.MethodPost, "/events", bytes.NewReader(b3))
	w3 := httptest.NewRecorder()
	h2.PostEvent(w3, req3)
	assert.Equal(t, http.StatusAccepted, w3.Code, "event must be accepted after backpressure clears, not treated as duplicate")
}

func TestGetMetrics_MissingEventName(t *testing.T) {
	h := NewMetricsHandler(nil)

	req := httptest.NewRequest(http.MethodGet, "/metrics?from=1&to=2", nil)
	w := httptest.NewRecorder()
	h.GetMetrics(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetMetrics_InvalidRange(t *testing.T) {
	h := NewMetricsHandler(nil)

	req := httptest.NewRequest(http.MethodGet, "/metrics?event_name=test&from=100&to=50", nil)
	w := httptest.NewRecorder()
	h.GetMetrics(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp dto.ErrorResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, "INVALID_RANGE", resp.Code)
}

func TestGetMetrics_InvalidGroupBy(t *testing.T) {
	h := NewMetricsHandler(nil)

	req := httptest.NewRequest(http.MethodGet, "/metrics?event_name=test&from=1&to=2&group_by=invalid", nil)
	w := httptest.NewRecorder()
	h.GetMetrics(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
