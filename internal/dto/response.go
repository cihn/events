package dto

// EventResponse is returned for single event ingestion.
type EventResponse struct {
	EventID string `json:"event_id" example:"a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4"`
	Status  string `json:"status" example:"accepted"`
}

// BulkEventResponse is returned for bulk event ingestion.
type BulkEventResponse struct {
	Accepted   int      `json:"accepted" example:"95"`
	Duplicates int      `json:"duplicates" example:"3"`
	Rejected   int      `json:"rejected" example:"5"`
	Errors     []string `json:"errors,omitempty"`
}

// ErrorResponse is a structured error payload.
type ErrorResponse struct {
	Error   string            `json:"error" example:"validation failed"`
	Code    string            `json:"code" example:"VALIDATION_ERROR"`
	Details map[string]string `json:"details,omitempty"`
}

// MetricsResponse is the analytics query result.
type MetricsResponse struct {
	EventName   string            `json:"event_name" example:"product_view"`
	From        int64             `json:"from" example:"1723400000"`
	To          int64             `json:"to" example:"1723500000"`
	TotalCount  uint64            `json:"total_count" example:"15432"`
	UniqueUsers uint64            `json:"unique_users" example:"8721"`
	Breakdown   []MetricBreakdown `json:"breakdown,omitempty"`
}

// MetricBreakdown represents a single aggregation bucket.
type MetricBreakdown struct {
	Key         string `json:"key" example:"web"`
	TotalCount  uint64 `json:"total_count" example:"9200"`
	UniqueUsers uint64 `json:"unique_users" example:"5100"`
}
