package handler

import (
	"log/slog"
	"net/http"
	"strconv"

	"events/internal/dto"
	"events/internal/service"
)

type MetricsHandler struct {
	svc *service.MetricsService
}

func NewMetricsHandler(svc *service.MetricsService) *MetricsHandler {
	return &MetricsHandler{svc: svc}
}

// GetMetrics godoc
// @Summary      Query aggregated event metrics
// @Description  Returns total count, unique users, and optional breakdown by channel or time bucket
// @Tags         metrics
// @Produce      json
// @Param        event_name  query     string  true   "Event name filter"
// @Param        from        query     int     true   "Start timestamp (unix seconds)"
// @Param        to          query     int     true   "End timestamp (unix seconds)"
// @Param        group_by    query     string  false  "Aggregation dimension: channel, hourly, daily"
// @Success      200  {object}  dto.MetricsResponse
// @Failure      400  {object}  dto.ErrorResponse
// @Failure      500  {object}  dto.ErrorResponse
// @Router       /metrics [get]
func (h *MetricsHandler) GetMetrics(w http.ResponseWriter, r *http.Request) {
	eventName := r.URL.Query().Get("event_name")
	if eventName == "" {
		writeJSON(w, http.StatusBadRequest, dto.ErrorResponse{
			Error: "event_name query parameter is required",
			Code:  "MISSING_PARAM",
		})
		return
	}

	from, err := strconv.ParseInt(r.URL.Query().Get("from"), 10, 64)
	if err != nil || from <= 0 {
		writeJSON(w, http.StatusBadRequest, dto.ErrorResponse{
			Error: "valid 'from' unix timestamp is required",
			Code:  "INVALID_PARAM",
		})
		return
	}

	to, err := strconv.ParseInt(r.URL.Query().Get("to"), 10, 64)
	if err != nil || to <= 0 {
		writeJSON(w, http.StatusBadRequest, dto.ErrorResponse{
			Error: "valid 'to' unix timestamp is required",
			Code:  "INVALID_PARAM",
		})
		return
	}

	if from >= to {
		writeJSON(w, http.StatusBadRequest, dto.ErrorResponse{
			Error: "'from' must be less than 'to'",
			Code:  "INVALID_RANGE",
		})
		return
	}

	groupBy := r.URL.Query().Get("group_by")
	switch groupBy {
	case "", "channel", "hourly", "daily":
	default:
		writeJSON(w, http.StatusBadRequest, dto.ErrorResponse{
			Error: "group_by must be one of: channel, hourly, daily",
			Code:  "INVALID_PARAM",
		})
		return
	}

	result, err := h.svc.GetMetrics(r.Context(), eventName, from, to, groupBy)
	if err != nil {
		slog.Error("metrics query failed",
			"event_name", eventName,
			"from", from,
			"to", to,
			"group_by", groupBy,
			"error", err,
		)
		writeJSON(w, http.StatusInternalServerError, dto.ErrorResponse{
			Error: "failed to query metrics",
			Code:  "INTERNAL_ERROR",
		})
		return
	}

	writeJSON(w, http.StatusOK, result)
}
