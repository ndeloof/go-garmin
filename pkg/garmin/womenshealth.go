package garmin

import (
	"context"
	"encoding/json"
	"fmt"
)

// WomensHealthService accesses periodichealth-service endpoints (menstrual
// cycle tracking, pregnancy).
type WomensHealthService struct{ c *Client }

// MenstrualDay returns the cycle day-view for one date.
func (s *WomensHealthService) MenstrualDay(ctx context.Context, date Date) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.c.getJSON(ctx, "/periodichealth-service/menstrualcycle/dayview/"+date.String(), nil, &raw)
	return raw, err
}

// MenstrualCalendar returns cycle calendar data over [start, end].
func (s *WomensHealthService) MenstrualCalendar(ctx context.Context, start, end Date) (json.RawMessage, error) {
	path := fmt.Sprintf("/periodichealth-service/menstrualcycle/calendar/%s/%s", start, end)
	var raw json.RawMessage
	err := s.c.getJSON(ctx, path, nil, &raw)
	return raw, err
}

// PregnancySummary returns the pregnancy snapshot.
func (s *WomensHealthService) PregnancySummary(ctx context.Context) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.c.getJSON(ctx, "/periodichealth-service/menstrualcycle/pregnancysnapshot", nil, &raw)
	return raw, err
}
