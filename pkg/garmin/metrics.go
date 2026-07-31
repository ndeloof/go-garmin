package garmin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// MetricsService accesses metrics-service and fitnessage-service endpoints
// (VO2max, training readiness/status, endurance & hill scores, race
// predictions…).
type MetricsService struct{ c *Client }

// MaxMetrics returns the day's VO2max / max-met metrics.
func (s *MetricsService) MaxMetrics(ctx context.Context, date Date) (json.RawMessage, error) {
	path := fmt.Sprintf("/metrics-service/metrics/maxmet/daily/%s/%s", date, date)
	var raw json.RawMessage
	err := s.c.getJSON(ctx, path, nil, &raw)
	return raw, err
}

// TrainingReadiness returns the day's training-readiness entries.
func (s *MetricsService) TrainingReadiness(ctx context.Context, date Date) ([]TrainingReadiness, error) {
	var list []TrainingReadiness
	err := s.c.getJSON(ctx, "/metrics-service/metrics/trainingreadiness/"+date.String(), nil, &list)
	return list, err
}

// TrainingReadiness is one training-readiness entry.
type TrainingReadiness struct {
	UserProfilePK int64           `json:"userProfilePK"`
	CalendarDate  Date            `json:"calendarDate"`
	Timestamp     string          `json:"timestamp"`
	Score         *int            `json:"score"`
	Level         string          `json:"level"`
	FeedbackShort string          `json:"feedbackShort"`
	InputContext  string          `json:"inputContext"`
	SleepScore    *int            `json:"sleepScore"`
	RecoveryTime  *int            `json:"recoveryTime"`
	AcwrFactor    json.RawMessage `json:"acwrFactorFeedback,omitempty"`
}

// MorningTrainingReadiness returns the after-wakeup reading when present,
// falling back to the first entry.
func (s *MetricsService) MorningTrainingReadiness(ctx context.Context, date Date) (*TrainingReadiness, error) {
	list, err := s.TrainingReadiness(ctx, date)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("%w: no training readiness data", ErrNotFound)
	}
	for i := range list {
		if list[i].InputContext == "AFTER_WAKEUP_RESET" {
			return &list[i], nil
		}
	}
	return &list[0], nil
}

// TrainingStatus returns the day's aggregated training status.
func (s *MetricsService) TrainingStatus(ctx context.Context, date Date) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.c.getJSON(ctx, "/metrics-service/metrics/trainingstatus/aggregated/"+date.String(), nil, &raw)
	return raw, err
}

// EnduranceScore returns the endurance score of one day.
func (s *MetricsService) EnduranceScore(ctx context.Context, date Date) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.c.getJSON(ctx, "/metrics-service/metrics/endurancescore", urlValues("calendarDate", date.String()), &raw)
	return raw, err
}

// EnduranceScoreRange returns weekly endurance-score stats over [start, end].
func (s *MetricsService) EnduranceScoreRange(ctx context.Context, start, end Date) (json.RawMessage, error) {
	q := url.Values{"startDate": {start.String()}, "endDate": {end.String()}, "aggregation": {"weekly"}}
	var raw json.RawMessage
	err := s.c.getJSON(ctx, "/metrics-service/metrics/endurancescore/stats", q, &raw)
	return raw, err
}

// HillScore returns the hill score of one day.
func (s *MetricsService) HillScore(ctx context.Context, date Date) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.c.getJSON(ctx, "/metrics-service/metrics/hillscore", urlValues("calendarDate", date.String()), &raw)
	return raw, err
}

// HillScoreRange returns daily hill-score stats over [start, end].
func (s *MetricsService) HillScoreRange(ctx context.Context, start, end Date) (json.RawMessage, error) {
	q := url.Values{"startDate": {start.String()}, "endDate": {end.String()}, "aggregation": {"daily"}}
	var raw json.RawMessage
	err := s.c.getJSON(ctx, "/metrics-service/metrics/hillscore/stats", q, &raw)
	return raw, err
}

// RacePredictions returns the latest race-time predictions.
func (s *MetricsService) RacePredictions(ctx context.Context) (json.RawMessage, error) {
	dn, err := s.c.DisplayName(ctx)
	if err != nil {
		return nil, err
	}
	var raw json.RawMessage
	err = s.c.getJSON(ctx, "/metrics-service/metrics/racepredictions/latest/"+dn, nil, &raw)
	return raw, err
}

// RacePredictionsRange returns daily or monthly race predictions over
// [start, end] (granularity "daily" or "monthly", range capped at 366 days).
func (s *MetricsService) RacePredictionsRange(ctx context.Context, start, end Date, granularity string) (json.RawMessage, error) {
	if granularity != "daily" && granularity != "monthly" {
		return nil, fmt.Errorf("garmin: granularity %q must be daily or monthly", granularity)
	}
	if start.DaysUntil(end) > 366 {
		return nil, fmt.Errorf("garmin: race predictions range is capped at 366 days")
	}
	dn, err := s.c.DisplayName(ctx)
	if err != nil {
		return nil, err
	}
	q := url.Values{"fromCalendarDate": {start.String()}, "toCalendarDate": {end.String()}}
	var raw json.RawMessage
	err = s.c.getJSON(ctx, fmt.Sprintf("/metrics-service/metrics/racepredictions/%s/%s", granularity, dn), q, &raw)
	return raw, err
}

// RunningTolerance returns running-tolerance stats over [start, end]
// (aggregation "daily" or "weekly").
func (s *MetricsService) RunningTolerance(ctx context.Context, start, end Date, aggregation string) (json.RawMessage, error) {
	if aggregation == "" {
		aggregation = "weekly"
	}
	if aggregation != "daily" && aggregation != "weekly" {
		return nil, fmt.Errorf("garmin: aggregation %q must be daily or weekly", aggregation)
	}
	q := url.Values{"startDate": {start.String()}, "endDate": {end.String()}, "aggregation": {aggregation}}
	var raw json.RawMessage
	err := s.c.getJSON(ctx, "/metrics-service/metrics/runningtolerance/stats", q, &raw)
	return raw, err
}

// FitnessAge returns the day's fitness-age payload.
func (s *MetricsService) FitnessAge(ctx context.Context, date Date) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.c.getJSON(ctx, "/fitnessage-service/fitnessage/"+date.String(), nil, &raw)
	return raw, err
}
