package garmin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// ActivityDetails is the sample/chart payload of one activity. Garmin returns
// it in columnar form: MetricDescriptors names the columns,
// ActivityDetailMetrics carries the rows. Use Series to pivot a metric into a
// plain slice.
type ActivityDetails struct {
	ActivityID            int64              `json:"activityId"`
	MeasurementCount      int                `json:"measurementCount"`
	MetricsCount          int                `json:"metricsCount"`
	MetricDescriptors     []MetricDescriptor `json:"metricDescriptors"`
	ActivityDetailMetrics []DetailMetricRow  `json:"activityDetailMetrics"`
	GeoPolylineDTO        json.RawMessage    `json:"geoPolylineDTO,omitempty"`
}

// MetricDescriptor names one column of the details payload.
type MetricDescriptor struct {
	MetricsIndex int    `json:"metricsIndex"`
	Key          string `json:"key"` // e.g. "directTimestamp", "sumDistance", "directHeartRate"
	Unit         struct {
		Key    string  `json:"key"`
		Factor float64 `json:"factor"`
	} `json:"unit"`
}

// DetailMetricRow is one sample row; entries may be null.
type DetailMetricRow struct {
	Metrics []*float64 `json:"metrics"`
}

// Common metric descriptor keys.
const (
	MetricTimestamp          = "directTimestamp" // ms epoch
	MetricSumDistance        = "sumDistance"     // cumulative meters
	MetricSpeed              = "directSpeed"     // m/s
	MetricElevation          = "directElevation" // meters
	MetricCorrectedElevation = "directCorrectedElevation"
	MetricHeartRate          = "directHeartRate" // bpm
	MetricLatitude           = "directLatitude"
	MetricLongitude          = "directLongitude"
)

// Series extracts the column named key as a flat slice (nil entries for null
// samples), or nil when the metric is absent.
func (d *ActivityDetails) Series(key string) []*float64 {
	idx := -1
	for _, md := range d.MetricDescriptors {
		if md.Key == key {
			idx = md.MetricsIndex
			break
		}
	}
	if idx < 0 {
		return nil
	}
	out := make([]*float64, 0, len(d.ActivityDetailMetrics))
	for _, row := range d.ActivityDetailMetrics {
		if idx < len(row.Metrics) {
			out = append(out, row.Metrics[idx])
		} else {
			out = append(out, nil)
		}
	}
	return out
}

// ActivityDetailsOptions bounds the sample resolution.
type ActivityDetailsOptions struct {
	MaxChartSize    int // number of chart samples (default 2000)
	MaxPolylineSize int // number of polyline points (default 4000)
}

// Details returns the columnar samples of an activity.
func (s *ActivitiesService) Details(ctx context.Context, activityID int64, opts *ActivityDetailsOptions) (*ActivityDetails, error) {
	chart, poly := 2000, 4000
	if opts != nil {
		if opts.MaxChartSize > 0 {
			chart = opts.MaxChartSize
		}
		if opts.MaxPolylineSize >= 0 {
			poly = opts.MaxPolylineSize
		}
	}
	q := url.Values{
		"maxChartSize":    {strconv.Itoa(chart)},
		"maxPolylineSize": {strconv.Itoa(poly)},
	}
	var d ActivityDetails
	if err := s.c.getJSON(ctx, fmt.Sprintf("/activity-service/activity/%d/details", activityID), q, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

// Splits returns the activity's lap splits.
func (s *ActivitiesService) Splits(ctx context.Context, activityID int64) (json.RawMessage, error) {
	return s.rawGet(ctx, activityID, "splits")
}

// TypedSplits returns the activity's typed splits.
func (s *ActivitiesService) TypedSplits(ctx context.Context, activityID int64) (json.RawMessage, error) {
	return s.rawGet(ctx, activityID, "typedsplits")
}

// SplitSummaries returns the activity's split summaries.
func (s *ActivitiesService) SplitSummaries(ctx context.Context, activityID int64) (json.RawMessage, error) {
	return s.rawGet(ctx, activityID, "split_summaries")
}

// Weather returns the weather observed during the activity.
func (s *ActivitiesService) Weather(ctx context.Context, activityID int64) (json.RawMessage, error) {
	return s.rawGet(ctx, activityID, "weather")
}

// HRTimeInZones returns the time spent in each heart-rate zone.
func (s *ActivitiesService) HRTimeInZones(ctx context.Context, activityID int64) (json.RawMessage, error) {
	return s.rawGet(ctx, activityID, "hrTimeInZones")
}

// PowerTimeInZones returns the time spent in each power zone.
func (s *ActivitiesService) PowerTimeInZones(ctx context.Context, activityID int64) (json.RawMessage, error) {
	return s.rawGet(ctx, activityID, "powerTimeInZones")
}

// ExerciseSets returns the exercise sets of a strength activity.
func (s *ActivitiesService) ExerciseSets(ctx context.Context, activityID int64) (json.RawMessage, error) {
	return s.rawGet(ctx, activityID, "exerciseSets")
}

// SetExerciseSets replaces the full exercise-set list of a strength activity.
// Garmin validates exercises[].category/.name against its FIT enum (a 400
// "Invalid Sub-Category Passed" means an unknown name).
func (s *ActivitiesService) SetExerciseSets(ctx context.Context, activityID int64, payload any) error {
	return s.c.Do(ctx, http.MethodPut, fmt.Sprintf("/activity-service/activity/%d/exerciseSets", activityID), nil, payload, nil)
}

// ExerciseSetType distinguishes a worked set from the rest between two sets,
// as returned in ExerciseSets and expected by SetTypedExerciseSets.
type ExerciseSetType string

const (
	ExerciseSetActive ExerciseSetType = "ACTIVE"
	ExerciseSetRest   ExerciseSetType = "REST"
)

// ExerciseSetExercise is one candidate exercise of a set. Garmin's own
// on-watch auto-detection lists several candidates with a Probability each; a
// manually logged set (this package's use case) normally carries exactly one,
// which should be left at its zero value — MarshalJSON defaults a zero
// Probability to 100 on write. A real auto-detected candidate is never
// reported at 0 (the lowest one still carries a nonzero share), so 0 is
// never a meaningful confidence score to send deliberately; treating it as
// "unset → certain" avoids a real footgun found the hard way: Garmin
// silently stores a literal 0 as "not confidently identified", and its own
// apps then decline to show the exercise icon/link for that set even though
// the category/name are correctly persisted and readable back via the API.
// Name is nil for a category with no specific movement (e.g. a plain
// PUSH_UP, never mapped to a named variant).
type ExerciseSetExercise struct {
	Category    string
	Name        *string
	Probability float64
}

func (e ExerciseSetExercise) MarshalJSON() ([]byte, error) {
	prob := e.Probability
	if prob == 0 {
		prob = 100
	}
	return json.Marshal(struct {
		Category    string  `json:"category"`
		Name        *string `json:"name"`
		Probability float64 `json:"probability"`
	}{
		Category:    e.Category,
		Name:        e.Name,
		Probability: prob,
	})
}

// ExerciseSet is one set — or the rest between two sets — of a strength
// activity's exerciseSets list, as read by ExerciseSets and written by
// SetTypedExerciseSets. Reverse-engineered from real Garmin Connect payloads
// (undocumented API): a REST set carries WeightGrams -1 and no exercises; an
// ACTIVE set carries WeightGrams 0 for a bodyweight movement, otherwise the
// load in GRAMS (not kg). RepetitionCount is nil for a REST set, or for an
// ACTIVE set with no counted reps (a held/timed movement — only Duration
// then matters).
type ExerciseSet struct {
	SetType         ExerciseSetType
	Duration        time.Duration
	RepetitionCount *int
	WeightGrams     float64
	Exercises       []ExerciseSetExercise
	// StartTime is the local wall-clock instant the set began. Nil for a
	// REST set — Garmin infers the gap from the surrounding ACTIVE sets'
	// StartTime + Duration.
	StartTime *time.Time
}

func (s ExerciseSet) MarshalJSON() ([]byte, error) {
	exercises := s.Exercises
	if exercises == nil {
		exercises = []ExerciseSetExercise{}
	}
	var startTime *string
	if s.StartTime != nil {
		t := localTimestamp(*s.StartTime)
		startTime = &t
	}
	return json.Marshal(struct {
		SetType         ExerciseSetType       `json:"setType"`
		Duration        float64               `json:"duration"`
		RepetitionCount *int                  `json:"repetitionCount"`
		Weight          float64               `json:"weight"`
		Exercises       []ExerciseSetExercise `json:"exercises"`
		StartTime       *string               `json:"startTime"`
	}{
		SetType:         s.SetType,
		Duration:        s.Duration.Seconds(),
		RepetitionCount: s.RepetitionCount,
		Weight:          s.WeightGrams,
		Exercises:       exercises,
		StartTime:       startTime,
	})
}

// SetTypedExerciseSets is a typed, validated convenience over
// SetExerciseSets: every ACTIVE set's exercises are checked against the FIT
// taxonomy (ValidExercise) before sending — Garmin accepts an unknown pair
// silently server-side (it just renders as a generic exercise on the web/app,
// same caveat as WorkoutStep), so this catches a typo/stale mapping at
// caller-visible cost instead.
func (s *ActivitiesService) SetTypedExerciseSets(ctx context.Context, activityID int64, sets []ExerciseSet) error {
	for i, st := range sets {
		for _, e := range st.Exercises {
			name := ""
			if e.Name != nil {
				name = *e.Name
			}
			if name != "" && !ValidExercise(e.Category, name) {
				return fmt.Errorf("garmin: exercise set %d: unknown (category, name) pair (%q, %q)", i, e.Category, name)
			}
		}
	}
	// activityId is required at the top level alongside exerciseSets — Garmin
	// rejects the write otherwise ("Activity ID should not be Null in the
	// Exercises Object"), even though it is redundant with the URL path.
	return s.SetExerciseSets(ctx, activityID, map[string]any{"activityId": activityID, "exerciseSets": sets})
}

func (s *ActivitiesService) rawGet(ctx context.Context, activityID int64, suffix string) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.c.getJSON(ctx, fmt.Sprintf("/activity-service/activity/%d/%s", activityID, suffix), nil, &raw)
	return raw, err
}
