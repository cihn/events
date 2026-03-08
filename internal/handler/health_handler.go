package handler

import (
	"context"
	"net/http"

	"events/internal/pipeline"
)

type Pinger interface {
	Ping(ctx context.Context) error
}

type HealthHandler struct {
	pinger   Pinger
	pipeline *pipeline.Pipeline
}

func NewHealthHandler(pinger Pinger, p *pipeline.Pipeline) *HealthHandler {
	return &HealthHandler{pinger: pinger, pipeline: p}
}

// Liveness godoc
// @Summary      Liveness probe
// @Tags         health
// @Success      200  {object}  map[string]string
// @Router       /health/live [get]
func (h *HealthHandler) Liveness(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Readiness godoc
// @Summary      Readiness probe
// @Description  Checks ClickHouse connectivity and returns pipeline stats
// @Tags         health
// @Success      200  {object}  map[string]any
// @Failure      503  {object}  map[string]string
// @Router       /health/ready [get]
func (h *HealthHandler) Readiness(w http.ResponseWriter, r *http.Request) {
	if err := h.pinger.Ping(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "not ready",
			"error":  err.Error(),
		})
		return
	}

	submitted, processed, dropped := h.pipeline.Stats()

	writeJSON(w, http.StatusOK, map[string]any{
		"status":       "ready",
		"queue_length": h.pipeline.QueueLen(),
		"submitted":    submitted,
		"processed":    processed,
		"dropped":      dropped,
	})
}
