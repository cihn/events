package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"events/internal/dto"
	"events/internal/service"
	"events/internal/validator"
)

type EventHandler struct {
	svc *service.EventService
}

func NewEventHandler(svc *service.EventService) *EventHandler {
	return &EventHandler{svc: svc}
}

// PostEvent godoc
// @Summary      Ingest a single event
// @Description  Validates and enqueues a single event for async persistence to ClickHouse
// @Tags         events
// @Accept       json
// @Produce      json
// @Param        event  body      dto.EventRequest  true  "Event payload"
// @Success      202    {object}  dto.EventResponse
// @Success      200    {object}  dto.EventResponse  "Duplicate event (idempotent)"
// @Failure      400    {object}  dto.ErrorResponse
// @Failure      413    {object}  dto.ErrorResponse
// @Failure      503    {object}  dto.ErrorResponse
// @Router       /events [post]
func (h *EventHandler) PostEvent(w http.ResponseWriter, r *http.Request) {
	var req dto.EventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeJSON(w, http.StatusRequestEntityTooLarge, dto.ErrorResponse{
				Error: "request body too large",
				Code:  "BODY_TOO_LARGE",
			})
			return
		}
		writeJSON(w, http.StatusBadRequest, dto.ErrorResponse{
			Error: "malformed JSON payload",
			Code:  "INVALID_JSON",
		})
		return
	}

	result := h.svc.Ingest(&req)

	if len(result.Errors) > 0 {
		details := make(map[string]string, len(result.Errors))
		for _, e := range result.Errors {
			details[e.Field] = e.Message
		}
		writeJSON(w, http.StatusBadRequest, dto.ErrorResponse{
			Error:   "validation failed",
			Code:    "VALIDATION_ERROR",
			Details: details,
		})
		return
	}

	if result.Duplicate {
		writeJSON(w, http.StatusOK, dto.EventResponse{
			EventID: result.EventID,
			Status:  "duplicate",
		})
		return
	}

	if result.Backpressure {
		w.Header().Set("Retry-After", "5")
		writeJSON(w, http.StatusServiceUnavailable, dto.ErrorResponse{
			Error: "service is at capacity, retry later",
			Code:  "BACKPRESSURE",
		})
		return
	}

	writeJSON(w, http.StatusAccepted, dto.EventResponse{
		EventID: result.EventID,
		Status:  "accepted",
	})
}

// PostBulkEvents godoc
// @Summary      Ingest events in bulk
// @Description  Validates and enqueues multiple events for async persistence
// @Tags         events
// @Accept       json
// @Produce      json
// @Param        events  body      dto.BulkEventRequest  true  "Bulk event payload"
// @Success      202     {object}  dto.BulkEventResponse
// @Failure      400     {object}  dto.ErrorResponse
// @Failure      413     {object}  dto.ErrorResponse
// @Failure      503     {object}  dto.ErrorResponse
// @Router       /events/bulk [post]
func (h *EventHandler) PostBulkEvents(w http.ResponseWriter, r *http.Request) {
	var req dto.BulkEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeJSON(w, http.StatusRequestEntityTooLarge, dto.ErrorResponse{
				Error: "request body too large",
				Code:  "BODY_TOO_LARGE",
			})
			return
		}
		writeJSON(w, http.StatusBadRequest, dto.ErrorResponse{
			Error: "malformed JSON payload",
			Code:  "INVALID_JSON",
		})
		return
	}

	if errs := validator.ValidateBulkRequest(&req); len(errs) > 0 {
		details := make(map[string]string, len(errs))
		for _, e := range errs {
			details[e.Field] = e.Message
		}
		writeJSON(w, http.StatusBadRequest, dto.ErrorResponse{
			Error:   "validation failed",
			Code:    "VALIDATION_ERROR",
			Details: details,
		})
		return
	}

	resp := dto.BulkEventResponse{}
	backpressure := false

	for i := range req.Events {
		result := h.svc.Ingest(&req.Events[i])

		if len(result.Errors) > 0 {
			resp.Rejected++
			if len(resp.Errors) < 10 {
				for _, e := range result.Errors {
					resp.Errors = append(resp.Errors, e.Error())
				}
			}
			continue
		}

		if result.Duplicate {
			resp.Duplicates++
			continue
		}

		if result.Backpressure {
			backpressure = true
			resp.Rejected++
			continue
		}

		resp.Accepted++
	}

	status := http.StatusAccepted
	if backpressure && resp.Accepted == 0 {
		w.Header().Set("Retry-After", "5")
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, resp)
}
