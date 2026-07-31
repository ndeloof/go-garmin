package garmin

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// MaxActivityPageSize is the largest page Garmin accepts on activity search.
const MaxActivityPageSize = 1000

// ActivitiesService accesses activitylist-service and activity-service
// endpoints.
type ActivitiesService struct{ c *Client }

// ActivityType identifies a Garmin activity type.
type ActivityType struct {
	TypeID       int64  `json:"typeId,omitempty"`
	TypeKey      string `json:"typeKey,omitempty"` // e.g. "running", "trail_running"
	ParentTypeID int64  `json:"parentTypeId,omitempty"`
}

// Activity is one entry of the activity list / detail payload. Metric units
// throughout: meters, seconds, m/s.
type Activity struct {
	ActivityID                            int64           `json:"activityId"`
	ActivityName                          string          `json:"activityName"`
	Description                           string          `json:"description,omitempty"`
	ActivityType                          ActivityType    `json:"activityType"`
	EventType                             *ActivityType   `json:"eventType,omitempty"`
	StartTimeGMT                          string          `json:"startTimeGMT"`   // "2006-01-02 15:04:05", naive UTC
	StartTimeLocal                        string          `json:"startTimeLocal"` // naive local time
	Distance                              float64         `json:"distance"`       // meters
	Duration                              float64         `json:"duration"`       // seconds (elapsed)
	ElapsedDuration                       float64         `json:"elapsedDuration,omitempty"`
	MovingDuration                        float64         `json:"movingDuration,omitempty"`
	ElevationGain                         *float64        `json:"elevationGain,omitempty"` // meters
	ElevationLoss                         *float64        `json:"elevationLoss,omitempty"`
	MinElevation                          *float64        `json:"minElevation,omitempty"`
	MaxElevation                          *float64        `json:"maxElevation,omitempty"`
	AverageSpeed                          *float64        `json:"averageSpeed,omitempty"` // m/s
	MaxSpeed                              *float64        `json:"maxSpeed,omitempty"`
	Calories                              *float64        `json:"calories,omitempty"`
	BmrCalories                           *float64        `json:"bmrCalories,omitempty"`
	AverageHR                             *float64        `json:"averageHR,omitempty"`
	MaxHR                                 *float64        `json:"maxHR,omitempty"`
	AverageRunningCadenceInStepsPerMinute *float64        `json:"averageRunningCadenceInStepsPerMinute,omitempty"`
	MaxRunningCadenceInStepsPerMinute     *float64        `json:"maxRunningCadenceInStepsPerMinute,omitempty"`
	AverageBikingCadenceInRevPerMinute    *float64        `json:"averageBikingCadenceInRevPerMinute,omitempty"`
	Steps                                 *int64          `json:"steps,omitempty"`
	AvgPower                              *float64        `json:"avgPower,omitempty"`
	MaxPower                              *float64        `json:"maxPower,omitempty"`
	NormPower                             *float64        `json:"normPower,omitempty"`
	TrainingEffect                        *float64        `json:"aerobicTrainingEffect,omitempty"`
	AnaerobicEffect                       *float64        `json:"anaerobicTrainingEffect,omitempty"`
	ActivityTrainingLoad                  *float64        `json:"activityTrainingLoad,omitempty"`
	VO2MaxValue                           *float64        `json:"vO2MaxValue,omitempty"`
	Rpe                                   *float64        `json:"rpe,omitempty"`
	PerceivedExertion                     *float64        `json:"perceivedExertion,omitempty"`
	DeviceID                              *int64          `json:"deviceId,omitempty"`
	Manufacturer                          string          `json:"manufacturer,omitempty"`
	LapCount                              *int            `json:"lapCount,omitempty"`
	HasPolyline                           bool            `json:"hasPolyline,omitempty"`
	StartLatitude                         *float64        `json:"startLatitude,omitempty"`
	StartLongitude                        *float64        `json:"startLongitude,omitempty"`
	LocationName                          string          `json:"locationName,omitempty"`
	OwnerID                               int64           `json:"ownerId,omitempty"`
	OwnerDisplayName                      string          `json:"ownerDisplayName,omitempty"`
	Favorite                              bool            `json:"favorite,omitempty"`
	Parent                                bool            `json:"parent,omitempty"`
	Purposeful                            bool            `json:"purposeful,omitempty"`
	SummaryDTO                            json.RawMessage `json:"summaryDTO,omitempty"` // present on Get, not List
}

// StartGMT parses StartTimeGMT as UTC.
func (a *Activity) StartGMT() (time.Time, error) { return parseGarminTime(a.StartTimeGMT) }

// ActivityListOptions filters an activity search.
type ActivityListOptions struct {
	ListOptions
	ActivityType string // typeKey filter, e.g. "running"
	Search       string // free-text search
	StartDate    Date   // optional lower bound
	EndDate      Date   // optional upper bound
	SortOrder    string // "asc" | "desc" (default)
}

func (o *ActivityListOptions) query(defaultLimit int) url.Values {
	start, limit := 0, defaultLimit
	q := url.Values{}
	if o != nil {
		start, limit = o.ListOptions.startLimit(defaultLimit)
		if o.ActivityType != "" {
			q.Set("activityType", o.ActivityType)
		}
		if o.Search != "" {
			q.Set("search", o.Search)
		}
		if !o.StartDate.IsZero() {
			q.Set("startDate", o.StartDate.String())
		}
		if !o.EndDate.IsZero() {
			q.Set("endDate", o.EndDate.String())
		}
		if o.SortOrder != "" {
			q.Set("sortOrder", o.SortOrder)
		}
	}
	q.Set("start", strconv.Itoa(start))
	q.Set("limit", strconv.Itoa(limit))
	return q
}

// List returns one page of the account's activities, most recent first.
func (s *ActivitiesService) List(ctx context.Context, opts *ActivityListOptions) ([]Activity, error) {
	q := opts.query(20)
	var raw json.RawMessage
	if err := s.c.getJSON(ctx, "/activitylist-service/activities/search/activities", q, &raw); err != nil {
		return nil, err
	}
	return decodeActivityList(raw)
}

// decodeActivityList tolerates both response shapes Garmin uses: a bare array
// or {"activityList": […]}.
func decodeActivityList(raw json.RawMessage) ([]Activity, error) {
	var list []Activity
	if err := json.Unmarshal(raw, &list); err == nil {
		return list, nil
	}
	var wrapper struct {
		ActivityList []Activity `json:"activityList"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, fmt.Errorf("garmin: decoding activity list: %w", err)
	}
	return wrapper.ActivityList, nil
}

// All iterates over every matching activity, paginating transparently. Break
// out of the loop to stop early.
func (s *ActivitiesService) All(ctx context.Context, opts *ActivityListOptions) iter.Seq2[Activity, error] {
	pageSize := 100
	if opts != nil && opts.Limit > 0 {
		pageSize = min(opts.Limit, MaxActivityPageSize)
	}
	return paged(pageSize, func(start, limit int) ([]Activity, error) {
		o := ActivityListOptions{}
		if opts != nil {
			o = *opts
		}
		o.Start, o.Limit = start, limit
		return s.List(ctx, &o)
	})
}

// Count returns the total number of activities on the account.
func (s *ActivitiesService) Count(ctx context.Context) (int, error) {
	var res struct {
		TotalCount int `json:"totalCount"`
	}
	if err := s.c.getJSON(ctx, "/activitylist-service/activities/count", nil, &res); err != nil {
		return 0, err
	}
	return res.TotalCount, nil
}

// Last returns the most recent activity, or ErrNotFound when there is none.
func (s *ActivitiesService) Last(ctx context.Context) (*Activity, error) {
	list, err := s.List(ctx, &ActivityListOptions{ListOptions: ListOptions{Limit: 1}})
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("%w: no activities", ErrNotFound)
	}
	return &list[0], nil
}

// Get returns one activity's full payload.
func (s *ActivitiesService) Get(ctx context.Context, activityID int64) (*Activity, error) {
	var a Activity
	if err := s.c.getJSON(ctx, fmt.Sprintf("/activity-service/activity/%d", activityID), nil, &a); err != nil {
		return nil, err
	}
	return &a, nil
}

// ForDate returns the activities/heart-rate snapshot for one day (the
// mobile-gateway "forDate" payload).
func (s *ActivitiesService) ForDate(ctx context.Context, date Date) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.c.getJSON(ctx, "/mobile-gateway/heartRate/forDate/"+date.String(), nil, &raw)
	return raw, err
}

// Types lists every Garmin activity type.
func (s *ActivitiesService) Types(ctx context.Context) ([]ActivityType, error) {
	var types []ActivityType
	if err := s.c.getJSON(ctx, "/activity-service/activity/activityTypes", nil, &types); err != nil {
		return nil, err
	}
	return types, nil
}

// SetName renames an activity.
func (s *ActivitiesService) SetName(ctx context.Context, activityID int64, name string) error {
	return s.put(ctx, activityID, map[string]any{"activityId": activityID, "activityName": name})
}

// SetDescription updates an activity's description.
func (s *ActivitiesService) SetDescription(ctx context.Context, activityID int64, description string) error {
	return s.put(ctx, activityID, map[string]any{"activityId": activityID, "description": description})
}

// SetType changes an activity's type.
func (s *ActivitiesService) SetType(ctx context.Context, activityID int64, t ActivityType) error {
	return s.put(ctx, activityID, map[string]any{"activityId": activityID, "activityTypeDTO": t})
}

func (s *ActivitiesService) put(ctx context.Context, activityID int64, body any) error {
	return s.c.Do(ctx, http.MethodPut, fmt.Sprintf("/activity-service/activity/%d", activityID), nil, body, nil)
}

// Delete removes an activity permanently.
func (s *ActivitiesService) Delete(ctx context.Context, activityID int64) error {
	return s.c.Do(ctx, http.MethodDelete, fmt.Sprintf("/activity-service/activity/%d", activityID), nil, nil, nil)
}

// ManualActivity describes an activity created by hand (no device file).
type ManualActivity struct {
	Name     string
	TypeKey  string    // e.g. "running"
	Start    time.Time // local wall-clock start
	TimeZone string    // unitKey, e.g. "Europe/Paris"
	Distance float64   // meters
	Duration float64   // seconds
}

// CreateManual creates a manual activity and returns its payload.
func (s *ActivitiesService) CreateManual(ctx context.Context, m ManualActivity) (*Activity, error) {
	payload := map[string]any{
		"activityTypeDTO":      map[string]any{"typeKey": m.TypeKey},
		"accessControlRuleDTO": map[string]any{"typeId": 2, "typeKey": "private"},
		"timeZoneUnitDTO":      map[string]any{"unitKey": m.TimeZone},
		"activityName":         m.Name,
		"metadataDTO":          map[string]any{"autoCalcCalories": true},
		"summaryDTO": map[string]any{
			"startTimeLocal": localTimestamp(m.Start),
			"distance":       m.Distance,
			"duration":       m.Duration,
		},
	}
	return s.CreateFromPayload(ctx, payload)
}

// CreateFromPayload creates an activity from a raw activity-service payload
// (full control over every field).
func (s *ActivitiesService) CreateFromPayload(ctx context.Context, payload any) (*Activity, error) {
	var a Activity
	if err := s.c.Do(ctx, http.MethodPost, "/activity-service/activity", nil, payload, &a); err != nil {
		return nil, err
	}
	return &a, nil
}
