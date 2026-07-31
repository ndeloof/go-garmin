package garmin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// MaxHydrationML bounds a single hydration log entry (absolute value).
const MaxHydrationML = 10000

// Hydration returns the day's hydration log.
func (s *WellnessService) Hydration(ctx context.Context, date Date) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.c.getJSON(ctx, "/usersummary-service/usersummary/hydration/daily/"+date.String(), nil, &raw)
	return raw, err
}

// AddHydration logs a hydration intake in milliliters (negative to correct),
// stamped at t (zero time = now).
func (s *WellnessService) AddHydration(ctx context.Context, valueML float64, t time.Time) (json.RawMessage, error) {
	if valueML > MaxHydrationML || valueML < -MaxHydrationML {
		return nil, fmt.Errorf("garmin: hydration value %v out of range (±%d ml)", valueML, MaxHydrationML)
	}
	if t.IsZero() {
		t = time.Now()
	}
	body := map[string]any{
		"calendarDate":   DateOf(t).String(),
		"timestampLocal": localTimestamp(t),
		"valueInML":      valueML,
	}
	var raw json.RawMessage
	err := s.c.Do(ctx, http.MethodPut, "/usersummary-service/usersummary/hydration/log", nil, body, &raw)
	return raw, err
}

// BloodPressureService accesses bloodpressure-service endpoints.
type BloodPressureService struct{ c *Client }

// Range returns blood-pressure measurements over [start, end].
func (s *BloodPressureService) Range(ctx context.Context, start, end Date) (json.RawMessage, error) {
	path := fmt.Sprintf("/bloodpressure-service/bloodpressure/range/%s/%s", start, end)
	var raw json.RawMessage
	err := s.c.getJSON(ctx, path, urlValues("includeAll", "true"), &raw)
	return raw, err
}

// BloodPressure is a manual blood-pressure measurement.
type BloodPressure struct {
	Systolic  int       // mmHg, 70–260
	Diastolic int       // mmHg, 40–150
	Pulse     int       // bpm, 20–250
	Time      time.Time // zero = now
	Notes     string
}

// Set records a manual blood-pressure measurement.
func (s *BloodPressureService) Set(ctx context.Context, m BloodPressure) (json.RawMessage, error) {
	if m.Systolic < 70 || m.Systolic > 260 {
		return nil, fmt.Errorf("garmin: systolic %d out of range 70-260", m.Systolic)
	}
	if m.Diastolic < 40 || m.Diastolic > 150 {
		return nil, fmt.Errorf("garmin: diastolic %d out of range 40-150", m.Diastolic)
	}
	if m.Pulse < 20 || m.Pulse > 250 {
		return nil, fmt.Errorf("garmin: pulse %d out of range 20-250", m.Pulse)
	}
	t := m.Time
	if t.IsZero() {
		t = time.Now()
	}
	body := map[string]any{
		"measurementTimestampLocal": localTimestamp(t),
		"measurementTimestampGMT":   gmtTimestamp(t),
		"systolic":                  m.Systolic,
		"diastolic":                 m.Diastolic,
		"pulse":                     m.Pulse,
		"sourceType":                "MANUAL",
		"notes":                     m.Notes,
	}
	var raw json.RawMessage
	err := s.c.Do(ctx, http.MethodPost, "/bloodpressure-service/bloodpressure", nil, body, &raw)
	return raw, err
}

// Delete removes a blood-pressure measurement by version and date.
func (s *BloodPressureService) Delete(ctx context.Context, version int64, date Date) error {
	path := fmt.Sprintf("/bloodpressure-service/bloodpressure/%s/%d", date, version)
	return s.c.Do(ctx, http.MethodDelete, path, nil, nil, nil)
}
