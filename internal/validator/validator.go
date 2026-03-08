package validator

import (
	"fmt"
	"time"

	"events/internal/dto"
)

const (
	maxTimestampFuture = 5 * time.Minute
	minTimestamp        = 1_000_000_000 // ~Sep 2001
	maxEventNameLen     = 256
	maxUserIDLen        = 256
	maxChannelLen       = 128
	maxCampaignIDLen    = 256
	maxTags             = 50
	maxTagLen           = 128
	maxMetadataKeys     = 100
	maxMetadataDepth    = 3
	maxBulkSize         = 20_000
)

type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (v ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", v.Field, v.Message)
}

func ValidateEventRequest(req *dto.EventRequest) []ValidationError {
	var errs []ValidationError

	if req.EventName == "" {
		errs = append(errs, ValidationError{Field: "event_name", Message: "is required"})
	} else if len(req.EventName) > maxEventNameLen {
		errs = append(errs, ValidationError{Field: "event_name", Message: fmt.Sprintf("exceeds max length %d", maxEventNameLen)})
	}

	if req.UserID == "" {
		errs = append(errs, ValidationError{Field: "user_id", Message: "is required"})
	} else if len(req.UserID) > maxUserIDLen {
		errs = append(errs, ValidationError{Field: "user_id", Message: fmt.Sprintf("exceeds max length %d", maxUserIDLen)})
	}

	if req.Timestamp == 0 {
		errs = append(errs, ValidationError{Field: "timestamp", Message: "is required"})
	} else if req.Timestamp < minTimestamp {
		errs = append(errs, ValidationError{Field: "timestamp", Message: "is too old or invalid"})
	} else if time.Unix(req.Timestamp, 0).After(time.Now().Add(maxTimestampFuture)) {
		errs = append(errs, ValidationError{Field: "timestamp", Message: "is too far in the future"})
	}

	if len(req.Channel) > maxChannelLen {
		errs = append(errs, ValidationError{Field: "channel", Message: fmt.Sprintf("exceeds max length %d", maxChannelLen)})
	}

	if len(req.CampaignID) > maxCampaignIDLen {
		errs = append(errs, ValidationError{Field: "campaign_id", Message: fmt.Sprintf("exceeds max length %d", maxCampaignIDLen)})
	}

	if len(req.Tags) > maxTags {
		errs = append(errs, ValidationError{Field: "tags", Message: fmt.Sprintf("exceeds max count %d", maxTags)})
	} else {
		for i, tag := range req.Tags {
			if len(tag) > maxTagLen {
				errs = append(errs, ValidationError{
					Field:   fmt.Sprintf("tags[%d]", i),
					Message: fmt.Sprintf("exceeds max length %d", maxTagLen),
				})
			}
		}
	}

	if len(req.Metadata) > maxMetadataKeys {
		errs = append(errs, ValidationError{Field: "metadata", Message: fmt.Sprintf("exceeds max key count %d", maxMetadataKeys)})
	} else if req.Metadata != nil && metadataDepth(req.Metadata, 0) > maxMetadataDepth {
		errs = append(errs, ValidationError{Field: "metadata", Message: fmt.Sprintf("exceeds max nesting depth %d", maxMetadataDepth)})
	}

	return errs
}

func metadataDepth(m map[string]any, current int) int {
	max := current + 1
	for _, v := range m {
		if nested, ok := v.(map[string]any); ok {
			if d := metadataDepth(nested, current+1); d > max {
				max = d
			}
		}
	}
	return max
}

func ValidateBulkRequest(req *dto.BulkEventRequest) []ValidationError {
	var errs []ValidationError

	if len(req.Events) == 0 {
		errs = append(errs, ValidationError{Field: "events", Message: "must contain at least one event"})
	}
	if len(req.Events) > maxBulkSize {
		errs = append(errs, ValidationError{Field: "events", Message: fmt.Sprintf("exceeds max bulk size %d", maxBulkSize)})
	}

	return errs
}
