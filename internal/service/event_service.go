package service

import (
	"encoding/json"
	"time"

	"events/internal/dedup"
	"events/internal/dto"
	"events/internal/model"
	"events/internal/pipeline"
	"events/internal/validator"
)

type EventService struct {
	pipeline *pipeline.Pipeline
	dedup    *dedup.Cache
}

func NewEventService(p *pipeline.Pipeline, d *dedup.Cache) *EventService {
	return &EventService{pipeline: p, dedup: d}
}

type IngestResult struct {
	EventID      string
	Duplicate    bool
	Backpressure bool
	Errors       []validator.ValidationError
}

func (s *EventService) Ingest(req *dto.EventRequest) IngestResult {
	if errs := validator.ValidateEventRequest(req); len(errs) > 0 {
		return IngestResult{Errors: errs}
	}

	eventID := dedup.GenerateEventID(req.EventName, req.UserID, req.Timestamp)

	if s.dedup.Check(eventID) {
		return IngestResult{EventID: eventID, Duplicate: true}
	}

	event := toModel(req, eventID)

	if !s.pipeline.Submit(event) {
		s.dedup.Remove(eventID)
		return IngestResult{EventID: eventID, Backpressure: true}
	}

	return IngestResult{EventID: eventID}
}

func toModel(req *dto.EventRequest, eventID string) *model.Event {
	metadataStr := "{}"
	if req.Metadata != nil {
		if b, err := json.Marshal(req.Metadata); err == nil {
			metadataStr = string(b)
		}
	}

	tags := req.Tags
	if tags == nil {
		tags = []string{}
	}

	return &model.Event{
		EventID:    eventID,
		EventName:  req.EventName,
		UserID:     req.UserID,
		Timestamp:  time.Unix(req.Timestamp, 0).UTC(),
		Channel:    req.Channel,
		CampaignID: req.CampaignID,
		Tags:       tags,
		Metadata:   metadataStr,
		IngestedAt: time.Now().UTC(),
	}
}
