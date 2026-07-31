package garmin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
)

// SummariesService accesses usersummary-service, wellness daily charts and
// fitnessstats-service endpoints.
type SummariesService struct{ c *Client }

// DailySummary is the daily user summary (steps, calories, stress, body
// battery, heart rate…). Only commonly-used fields are typed; Raw carries the
// full payload.
type DailySummary struct {
	UserProfileID                    int64           `json:"userProfileId"`
	CalendarDate                     Date            `json:"calendarDate"`
	TotalSteps                       *int            `json:"totalSteps"`
	DailyStepGoal                    *int            `json:"dailyStepGoal"`
	TotalDistanceMeters              *float64        `json:"totalDistanceMeters"`
	TotalKilocalories                *float64        `json:"totalKilocalories"`
	ActiveKilocalories               *float64        `json:"activeKilocalories"`
	BmrKilocalories                  *float64        `json:"bmrKilocalories"`
	FloorsAscended                   *float64        `json:"floorsAscended"`
	FloorsDescended                  *float64        `json:"floorsDescended"`
	MinHeartRate                     *int            `json:"minHeartRate"`
	MaxHeartRate                     *int            `json:"maxHeartRate"`
	RestingHeartRate                 *int            `json:"restingHeartRate"`
	LastSevenDaysAvgRestingHeartRate *int            `json:"lastSevenDaysAvgRestingHeartRate"`
	AverageStressLevel               *int            `json:"averageStressLevel"`
	MaxStressLevel                   *int            `json:"maxStressLevel"`
	StressDuration                   *int            `json:"stressDuration"`
	BodyBatteryChargedValue          *int            `json:"bodyBatteryChargedValue"`
	BodyBatteryDrainedValue          *int            `json:"bodyBatteryDrainedValue"`
	BodyBatteryHighestValue          *int            `json:"bodyBatteryHighestValue"`
	BodyBatteryLowestValue           *int            `json:"bodyBatteryLowestValue"`
	BodyBatteryMostRecentValue       *int            `json:"bodyBatteryMostRecentValue"`
	ModerateIntensityMinutes         *int            `json:"moderateIntensityMinutes"`
	VigorousIntensityMinutes         *int            `json:"vigorousIntensityMinutes"`
	IntensityMinutesGoal             *int            `json:"userIntensityMinutesGoal"`
	SleepingSeconds                  *int            `json:"sleepingSeconds"`
	PrivacyProtected                 bool            `json:"privacyProtected"`
	Raw                              json.RawMessage `json:"-"`
}

// Daily returns the daily summary for one calendar date.
func (s *SummariesService) Daily(ctx context.Context, date Date) (*DailySummary, error) {
	dn, err := s.c.DisplayName(ctx)
	if err != nil {
		return nil, err
	}
	q := url.Values{"calendarDate": {date.String()}}
	var raw json.RawMessage
	if err := s.c.getJSON(ctx, "/usersummary-service/usersummary/daily/"+dn, q, &raw); err != nil {
		return nil, err
	}
	var ds DailySummary
	if err := json.Unmarshal(raw, &ds); err != nil {
		return nil, err
	}
	ds.Raw = raw
	// Garmin serves an empty, privacy-protected summary instead of a 401
	// when the session lost access to the account data.
	if ds.PrivacyProtected {
		return nil, fmt.Errorf("%w: daily summary is privacy protected", ErrUnauthorized)
	}
	return &ds, nil
}

// StepsChart returns the intraday steps chart (15-minute buckets).
func (s *SummariesService) StepsChart(ctx context.Context, date Date) (json.RawMessage, error) {
	dn, err := s.c.DisplayName(ctx)
	if err != nil {
		return nil, err
	}
	var raw json.RawMessage
	err = s.c.getJSON(ctx, "/wellness-service/wellness/dailySummaryChart/"+dn, url.Values{"date": {date.String()}}, &raw)
	return raw, err
}

// Floors returns the floors climbed chart for one day.
func (s *SummariesService) Floors(ctx context.Context, date Date) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.c.getJSON(ctx, "/wellness-service/wellness/floorsChartData/daily/"+date.String(), nil, &raw)
	if err == nil && len(raw) == 0 {
		return nil, errors.New("garmin: no floors data received")
	}
	return raw, err
}

// DailySteps is one day of the steps stats range.
type DailySteps struct {
	CalendarDate  Date     `json:"calendarDate"`
	TotalSteps    *int     `json:"totalSteps"`
	TotalDistance *float64 `json:"totalDistance"` // meters
	StepGoal      *int     `json:"stepGoal"`
}

// DailySteps returns daily step totals over [start, end]. Garmin caps the
// window at 28 days per request; wider ranges are split and concatenated
// transparently.
func (s *SummariesService) DailySteps(ctx context.Context, start, end Date) ([]DailySteps, error) {
	var out []DailySteps
	for _, r := range chunkDateRange(start, end, 28) {
		var chunk []DailySteps
		path := fmt.Sprintf("/usersummary-service/stats/steps/daily/%s/%s", r.start, r.end)
		if err := s.c.getJSON(ctx, path, nil, &chunk); err != nil {
			return nil, err
		}
		out = append(out, chunk...)
	}
	return out, nil
}

// WeeklySteps returns weekly step aggregates for the given number of weeks
// ending at end.
func (s *SummariesService) WeeklySteps(ctx context.Context, end Date, weeks int) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.c.getJSON(ctx, fmt.Sprintf("/usersummary-service/stats/steps/weekly/%s/%d", end, weeks), nil, &raw)
	return raw, err
}

// WeeklyStress returns weekly stress aggregates for the given number of weeks
// ending at end.
func (s *SummariesService) WeeklyStress(ctx context.Context, end Date, weeks int) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.c.getJSON(ctx, fmt.Sprintf("/usersummary-service/stats/stress/weekly/%s/%d", end, weeks), nil, &raw)
	return raw, err
}

// WeeklyIntensityMinutes returns weekly intensity-minutes aggregates over
// [start, end].
func (s *SummariesService) WeeklyIntensityMinutes(ctx context.Context, start, end Date) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.c.getJSON(ctx, fmt.Sprintf("/usersummary-service/stats/im/weekly/%s/%s", start, end), nil, &raw)
	return raw, err
}

// ProgressMetric selects the metric aggregated by ProgressSummary.
type ProgressMetric string

const (
	ProgressDistance       ProgressMetric = "distance"
	ProgressDuration       ProgressMetric = "duration"
	ProgressMovingDuration ProgressMetric = "movingDuration"
	ProgressElevationGain  ProgressMetric = "elevationGain"
)

// ProgressSummary returns fitness-stats aggregates between two dates,
// optionally grouped by parent activity type.
func (s *SummariesService) ProgressSummary(ctx context.Context, start, end Date, metric ProgressMetric, groupByActivities bool) (json.RawMessage, error) {
	q := url.Values{
		"startDate":                 {start.String()},
		"endDate":                   {end.String()},
		"aggregation":               {"lifetime"},
		"groupByParentActivityType": {strconv.FormatBool(groupByActivities)},
		"metric":                    {string(metric)},
	}
	var raw json.RawMessage
	err := s.c.getJSON(ctx, "/fitnessstats-service/activity", q, &raw)
	return raw, err
}
