package garmin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
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

func (s *ActivitiesService) rawGet(ctx context.Context, activityID int64, suffix string) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.c.getJSON(ctx, fmt.Sprintf("/activity-service/activity/%d/%s", activityID, suffix), nil, &raw)
	return raw, err
}
