package garmin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// WorkoutsService accesses workout-service, calendar-service and
// trainingplan-service endpoints.
type WorkoutsService struct{ c *Client }

// List returns one page of the account's structured workouts.
func (s *WorkoutsService) List(ctx context.Context, opts *ListOptions) ([]json.RawMessage, error) {
	start, limit := opts.startLimit(100)
	q := url.Values{"start": {strconv.Itoa(start)}, "limit": {strconv.Itoa(limit)}}
	var list []json.RawMessage
	err := s.c.getJSON(ctx, "/workout-service/workouts", q, &list)
	return list, err
}

// Get returns one workout's full payload.
func (s *WorkoutsService) Get(ctx context.Context, workoutID int64) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.c.getJSON(ctx, fmt.Sprintf("/workout-service/workout/%d", workoutID), nil, &raw)
	return raw, err
}

// Create uploads a workout payload (the workout-service JSON format; see the
// python-garminconnect workout builders for the shape) and returns the
// created workout.
func (s *WorkoutsService) Create(ctx context.Context, workout any) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.c.Do(ctx, http.MethodPost, "/workout-service/workout", nil, workout, &raw)
	return raw, err
}

// Update replaces a workout entirely.
func (s *WorkoutsService) Update(ctx context.Context, workoutID int64, workout map[string]any) error {
	workout["workoutId"] = workoutID
	return s.c.Do(ctx, http.MethodPut, fmt.Sprintf("/workout-service/workout/%d", workoutID), nil, workout, nil)
}

// Delete removes a workout.
func (s *WorkoutsService) Delete(ctx context.Context, workoutID int64) error {
	return s.c.Do(ctx, http.MethodDelete, fmt.Sprintf("/workout-service/workout/%d", workoutID), nil, nil, nil)
}

// DownloadFIT downloads a workout as a FIT file.
func (s *WorkoutsService) DownloadFIT(ctx context.Context, workoutID int64) ([]byte, error) {
	return s.c.download(ctx, fmt.Sprintf("/workout-service/workout/FIT/%d", workoutID), nil)
}

// Schedule puts a workout on the calendar at the given date and returns the
// scheduled-workout payload.
func (s *WorkoutsService) Schedule(ctx context.Context, workoutID int64, date Date) (json.RawMessage, error) {
	body := map[string]string{"date": date.String()}
	var raw json.RawMessage
	err := s.c.Do(ctx, http.MethodPost, fmt.Sprintf("/workout-service/schedule/%d", workoutID), nil, body, &raw)
	return raw, err
}

// Unschedule removes a scheduled workout from the calendar.
func (s *WorkoutsService) Unschedule(ctx context.Context, scheduleID int64) error {
	return s.c.Do(ctx, http.MethodDelete, fmt.Sprintf("/workout-service/schedule/%d", scheduleID), nil, nil, nil)
}

// Scheduled returns one scheduled workout.
func (s *WorkoutsService) Scheduled(ctx context.Context, scheduleID int64) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.c.getJSON(ctx, fmt.Sprintf("/workout-service/schedule/%d", scheduleID), nil, &raw)
	return raw, err
}

// Calendar returns the calendar of one month (year + 1-based month; the
// Garmin URL uses 0-based months, converted here).
func (s *WorkoutsService) Calendar(ctx context.Context, year int, month int) (json.RawMessage, error) {
	if month < 1 || month > 12 {
		return nil, fmt.Errorf("garmin: month %d out of range 1-12", month)
	}
	var raw json.RawMessage
	err := s.c.getJSON(ctx, fmt.Sprintf("/calendar-service/year/%d/month/%d", year, month-1), nil, &raw)
	return raw, err
}

// PushToDevice queues a workout for delivery to a device (device message).
func (s *WorkoutsService) PushToDevice(ctx context.Context, workoutID, deviceID int64) (json.RawMessage, error) {
	var workoutName string
	if wk, err := s.Get(ctx, workoutID); err == nil {
		var meta struct {
			WorkoutName string `json:"workoutName"`
		}
		_ = json.Unmarshal(wk, &meta)
		workoutName = meta.WorkoutName
	}
	body := []map[string]any{{
		"deviceId":    deviceID,
		"messageUrl":  fmt.Sprintf("workout-service/workout/FIT/%d", workoutID),
		"messageType": "workouts",
		"groupName":   nil,
		"messageName": workoutName,
		"priority":    1,
		"fileType":    "FIT",
		"metaDataId":  workoutID,
	}}
	var raw json.RawMessage
	err := s.c.Do(ctx, http.MethodPost, "/device-service/devicemessage/messages", nil, body, &raw)
	return raw, err
}

// TrainingPlans returns the account's training plans.
func (s *WorkoutsService) TrainingPlans(ctx context.Context) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.c.getJSON(ctx, "/trainingplan-service/trainingplan/plans", nil, &raw)
	return raw, err
}

// TrainingPlan returns one phased training plan.
func (s *WorkoutsService) TrainingPlan(ctx context.Context, planID int64) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.c.getJSON(ctx, fmt.Sprintf("/trainingplan-service/trainingplan/phased/%d", planID), nil, &raw)
	return raw, err
}

// AdaptiveTrainingPlan returns one adaptive (Garmin-coach) training plan.
func (s *WorkoutsService) AdaptiveTrainingPlan(ctx context.Context, planID int64) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.c.getJSON(ctx, fmt.Sprintf("/trainingplan-service/trainingplan/fbt-adaptive/%d", planID), nil, &raw)
	return raw, err
}
