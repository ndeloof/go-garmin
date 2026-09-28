package garmin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

// WellnessService accesses wellness-service, hrv-service, userstats-service
// and lifestylelogging-service endpoints.
type WellnessService struct{ c *Client }

// HeartRateDaily is the daily heart-rate payload.
type HeartRateDaily struct {
	UserProfilePK                    int64 `json:"userProfilePK"`
	CalendarDate                     Date  `json:"calendarDate"`
	MaxHeartRate                     *int  `json:"maxHeartRate"`
	MinHeartRate                     *int  `json:"minHeartRate"`
	RestingHeartRate                 *int  `json:"restingHeartRate"`
	LastSevenDaysAvgRestingHeartRate *int  `json:"lastSevenDaysAvgRestingHeartRate"`
	// HeartRateValues holds [timestampMillis, bpm] pairs; bpm may be null.
	HeartRateValues [][]*float64 `json:"heartRateValues"`
}

// HeartRates returns the day's heart-rate samples and aggregates.
func (s *WellnessService) HeartRates(ctx context.Context, date Date) (*HeartRateDaily, error) {
	dn, err := s.c.DisplayName(ctx)
	if err != nil {
		return nil, err
	}
	var hr HeartRateDaily
	if err := s.c.getJSON(ctx, "/wellness-service/wellness/dailyHeartRate/"+dn, url.Values{"date": {date.String()}}, &hr); err != nil {
		return nil, err
	}
	return &hr, nil
}

// RestingHeartRateDay returns the daily resting-heart-rate wellness metric
// (metricId 60).
func (s *WellnessService) RestingHeartRateDay(ctx context.Context, date Date) (json.RawMessage, error) {
	dn, err := s.c.DisplayName(ctx)
	if err != nil {
		return nil, err
	}
	q := url.Values{
		"fromDate":  {date.String()},
		"untilDate": {date.String()},
		"metricId":  {"60"},
	}
	var raw json.RawMessage
	err = s.c.getJSON(ctx, "/userstats-service/wellness/daily/"+dn, q, &raw)
	return raw, err
}

// HRV returns the day's heart-rate-variability payload.
func (s *WellnessService) HRV(ctx context.Context, date Date) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.c.getJSON(ctx, "/hrv-service/hrv/"+date.String(), nil, &raw)
	return raw, err
}

// HRV qualitative status values (hrvSummary.status in the raw payload).
// NONE means Garmin has not accumulated enough baseline data yet to classify
// the reading — distinct from an absent/errored call.
const (
	HRVStatusBalanced   = "BALANCED"
	HRVStatusUnbalanced = "UNBALANCED"
	HRVStatusLow        = "LOW"
	HRVStatusPoor       = "POOR"
	HRVStatusNone       = "NONE"
)

// HRVStatus is Garmin's day-level HRV classification, parsed from the
// "hrvSummary" object of the same payload HRV returns raw.
//
// Schema based on Garmin Connect's publicly documented, community
// reverse-engineered structure (the same one python-garminconnect's
// get_hrv_data exposes) — {"hrvSummary": {"status": "...", "weeklyAvg": N,
// "lastNightAvg": N}} — NOT yet confirmed against a live call from this
// library's own test suite (no linked test account was available at
// authoring time). Callers should treat an unrecognised Status value the
// same as HRVStatusNone (unknown, not degraded) until this is verified
// against a real account and this comment can be updated.
type HRVStatus struct {
	Status       string `json:"status"`
	WeeklyAvg    *int   `json:"weeklyAvg"`
	LastNightAvg *int   `json:"lastNightAvg"`
}

// HRVSummary returns the day's HRV status classification. Returns a zero
// HRVStatus (Status == "") rather than an error when the payload has no
// "hrvSummary" object (e.g. a device without HRV support) — callers already
// have to handle HRVStatusNone as "no real signal", so an empty Status folds
// into that same handling instead of needing its own error path.
func (s *WellnessService) HRVSummary(ctx context.Context, date Date) (*HRVStatus, error) {
	raw, err := s.HRV(ctx, date)
	if err != nil {
		return nil, err
	}
	var payload struct {
		HRVSummary HRVStatus `json:"hrvSummary"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	return &payload.HRVSummary, nil
}

// SleepData is the daily sleep payload; the full detail (sleep levels,
// movement…) stays in Raw.
type SleepData struct {
	DailySleepDTO struct {
		ID                      int64           `json:"id"`
		CalendarDate            Date            `json:"calendarDate"`
		SleepTimeSeconds        *int            `json:"sleepTimeSeconds"`
		NapTimeSeconds          *int            `json:"napTimeSeconds"`
		SleepStartTimestampGMT  *int64          `json:"sleepStartTimestampGMT"` // ms epoch
		SleepEndTimestampGMT    *int64          `json:"sleepEndTimestampGMT"`
		DeepSleepSeconds        *int            `json:"deepSleepSeconds"`
		LightSleepSeconds       *int            `json:"lightSleepSeconds"`
		RemSleepSeconds         *int            `json:"remSleepSeconds"`
		AwakeSleepSeconds       *int            `json:"awakeSleepSeconds"`
		AvgSleepStress          *float64        `json:"avgSleepStress"`
		AverageSpO2Value        *float64        `json:"averageSpO2Value"`
		AverageRespirationValue *float64        `json:"averageRespirationValue"`
		SleepScores             json.RawMessage `json:"sleepScores,omitempty"`
	} `json:"dailySleepDTO"`
	RestingHeartRate *int            `json:"restingHeartRate"`
	Raw              json.RawMessage `json:"-"`
}

// Sleep returns the night's sleep data for a calendar date.
func (s *WellnessService) Sleep(ctx context.Context, date Date) (*SleepData, error) {
	dn, err := s.c.DisplayName(ctx)
	if err != nil {
		return nil, err
	}
	q := url.Values{"date": {date.String()}, "nonSleepBufferMinutes": {"60"}}
	var raw json.RawMessage
	if err := s.c.getJSON(ctx, "/wellness-service/wellness/dailySleepData/"+dn, q, &raw); err != nil {
		return nil, err
	}
	var sd SleepData
	if err := json.Unmarshal(raw, &sd); err != nil {
		return nil, err
	}
	sd.Raw = raw
	return &sd, nil
}

// Stress returns the all-day stress payload.
func (s *WellnessService) Stress(ctx context.Context, date Date) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.c.getJSON(ctx, "/wellness-service/wellness/dailyStress/"+date.String(), nil, &raw)
	return raw, err
}

// DailyEvents returns the day's wellness events.
func (s *WellnessService) DailyEvents(ctx context.Context, date Date) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.c.getJSON(ctx, "/wellness-service/wellness/dailyEvents", url.Values{"calendarDate": {date.String()}}, &raw)
	return raw, err
}

// BodyBattery returns daily body-battery reports over [start, end].
func (s *WellnessService) BodyBattery(ctx context.Context, start, end Date) (json.RawMessage, error) {
	q := url.Values{"startDate": {start.String()}, "endDate": {end.String()}}
	var raw json.RawMessage
	err := s.c.getJSON(ctx, "/wellness-service/wellness/bodyBattery/reports/daily", q, &raw)
	return raw, err
}

// BodyBatteryEvents returns the day's body-battery events (sleep, activity,
// stress impacts).
func (s *WellnessService) BodyBatteryEvents(ctx context.Context, date Date) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.c.getJSON(ctx, "/wellness-service/wellness/bodyBattery/events/"+date.String(), nil, &raw)
	return raw, err
}

// Respiration returns the day's respiration payload.
func (s *WellnessService) Respiration(ctx context.Context, date Date) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.c.getJSON(ctx, "/wellness-service/wellness/daily/respiration/"+date.String(), nil, &raw)
	return raw, err
}

// SpO2 returns the day's pulse-ox payload.
func (s *WellnessService) SpO2(ctx context.Context, date Date) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.c.getJSON(ctx, "/wellness-service/wellness/daily/spo2/"+date.String(), nil, &raw)
	return raw, err
}

// IntensityMinutes returns the day's intensity-minutes payload.
func (s *WellnessService) IntensityMinutes(ctx context.Context, date Date) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.c.getJSON(ctx, "/wellness-service/wellness/daily/im/"+date.String(), nil, &raw)
	return raw, err
}

// LifestyleLog returns the day's lifestyle logging entries.
func (s *WellnessService) LifestyleLog(ctx context.Context, date Date) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.c.getJSON(ctx, "/lifestylelogging-service/dailyLog/"+date.String(), nil, &raw)
	return raw, err
}

// RequestReload asks Garmin to reload wellness epoch data for an old date
// (data older than the retention window is served only after such a reload).
func (s *WellnessService) RequestReload(ctx context.Context, date Date) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.c.Do(ctx, http.MethodPost, "/wellness-service/wellness/epoch/request/"+date.String(), nil, nil, &raw)
	return raw, err
}
