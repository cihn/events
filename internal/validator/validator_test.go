package validator

import (
	"strings"
	"testing"
	"time"

	"events/internal/dto"

	"github.com/stretchr/testify/assert"
)

func validRequest() *dto.EventRequest {
	return &dto.EventRequest{
		EventName:  "product_view",
		UserID:     "user_123",
		Timestamp:  time.Now().Unix(),
		Channel:    "web",
		CampaignID: "cmp_987",
		Tags:       []string{"electronics"},
		Metadata:   map[string]any{"product_id": "prod-789"},
	}
}

func TestValidateEventRequest_Valid(t *testing.T) {
	errs := ValidateEventRequest(validRequest())
	assert.Empty(t, errs)
}

func TestValidateEventRequest_MissingEventName(t *testing.T) {
	req := validRequest()
	req.EventName = ""
	errs := ValidateEventRequest(req)
	assert.Len(t, errs, 1)
	assert.Equal(t, "event_name", errs[0].Field)
}

func TestValidateEventRequest_MissingUserID(t *testing.T) {
	req := validRequest()
	req.UserID = ""
	errs := ValidateEventRequest(req)
	assert.Len(t, errs, 1)
	assert.Equal(t, "user_id", errs[0].Field)
}

func TestValidateEventRequest_MissingTimestamp(t *testing.T) {
	req := validRequest()
	req.Timestamp = 0
	errs := ValidateEventRequest(req)
	assert.Len(t, errs, 1)
	assert.Equal(t, "timestamp", errs[0].Field)
}

func TestValidateEventRequest_TimestampTooOld(t *testing.T) {
	req := validRequest()
	req.Timestamp = 100
	errs := ValidateEventRequest(req)
	assert.Len(t, errs, 1)
	assert.Contains(t, errs[0].Message, "too old")
}

func TestValidateEventRequest_TimestampFuture(t *testing.T) {
	req := validRequest()
	req.Timestamp = time.Now().Add(10 * time.Minute).Unix()
	errs := ValidateEventRequest(req)
	assert.Len(t, errs, 1)
	assert.Contains(t, errs[0].Message, "future")
}

func TestValidateEventRequest_MultipleErrors(t *testing.T) {
	req := &dto.EventRequest{}
	errs := ValidateEventRequest(req)
	assert.GreaterOrEqual(t, len(errs), 3)
}

func TestValidateEventRequest_EventNameTooLong(t *testing.T) {
	req := validRequest()
	req.EventName = strings.Repeat("a", 257)
	errs := ValidateEventRequest(req)
	assert.Len(t, errs, 1)
	assert.Equal(t, "event_name", errs[0].Field)
}

func TestValidateEventRequest_TooManyTags(t *testing.T) {
	req := validRequest()
	req.Tags = make([]string, 51)
	for i := range req.Tags {
		req.Tags[i] = "tag"
	}
	errs := ValidateEventRequest(req)
	assert.Len(t, errs, 1)
	assert.Equal(t, "tags", errs[0].Field)
}

func TestValidateEventRequest_TagTooLong(t *testing.T) {
	req := validRequest()
	req.Tags = []string{strings.Repeat("x", 129)}
	errs := ValidateEventRequest(req)
	assert.Len(t, errs, 1)
	assert.Equal(t, "tags[0]", errs[0].Field)
}

func TestValidateBulkRequest_Empty(t *testing.T) {
	req := &dto.BulkEventRequest{}
	errs := ValidateBulkRequest(req)
	assert.Len(t, errs, 1)
	assert.Equal(t, "events", errs[0].Field)
}

func TestValidateBulkRequest_TooLarge(t *testing.T) {
	req := &dto.BulkEventRequest{
		Events: make([]dto.EventRequest, 20001),
	}
	errs := ValidateBulkRequest(req)
	assert.Len(t, errs, 1)
	assert.Contains(t, errs[0].Message, "max bulk size")
}

func TestValidateBulkRequest_Valid(t *testing.T) {
	req := &dto.BulkEventRequest{
		Events: []dto.EventRequest{*validRequest()},
	}
	errs := ValidateBulkRequest(req)
	assert.Empty(t, errs)
}
