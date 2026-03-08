package model

import "time"

type Event struct {
	EventID    string
	EventName  string
	UserID     string
	Timestamp  time.Time
	Channel    string
	CampaignID string
	Tags       []string
	Metadata   string // JSON string
	IngestedAt time.Time
}
