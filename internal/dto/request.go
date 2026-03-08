package dto

// EventRequest represents a single event ingestion payload.
type EventRequest struct {
	EventName  string         `json:"event_name" example:"product_view"`
	UserID     string         `json:"user_id" example:"user_123"`
	Timestamp  int64          `json:"timestamp" example:"1723475612"`
	Channel    string         `json:"channel,omitempty" example:"web"`
	CampaignID string         `json:"campaign_id,omitempty" example:"cmp_987"`
	Tags       []string       `json:"tags,omitempty" example:"electronics,homepage"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// BulkEventRequest represents a bulk event ingestion payload.
type BulkEventRequest struct {
	Events []EventRequest `json:"events"`
}
