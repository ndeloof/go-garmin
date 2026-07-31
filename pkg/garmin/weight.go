package garmin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/ndeloof/go-garmin/internal/fitenc"
)

// WeightService accesses weight-service endpoints.
type WeightService struct{ c *Client }

// WeightUnit is the unit of a manual weigh-in.
type WeightUnit string

const (
	Kilograms WeightUnit = "kg"
	Pounds    WeightUnit = "lbs"
)

// BodyComposition returns weight/composition aggregates over [start, end]
// (the dateRange payload with totalAverage).
func (s *WeightService) BodyComposition(ctx context.Context, start, end Date) (json.RawMessage, error) {
	q := url.Values{"startDate": {start.String()}, "endDate": {end.String()}}
	var raw json.RawMessage
	err := s.c.getJSON(ctx, "/weight-service/weight/dateRange", q, &raw)
	return raw, err
}

// WeighIns returns individual weigh-ins over [start, end].
func (s *WeightService) WeighIns(ctx context.Context, start, end Date) (json.RawMessage, error) {
	path := fmt.Sprintf("/weight-service/weight/range/%s/%s", start, end)
	var raw json.RawMessage
	err := s.c.getJSON(ctx, path, urlValues("includeAll", "true"), &raw)
	return raw, err
}

// DailyWeighIns returns the weigh-ins of one calendar date.
func (s *WeightService) DailyWeighIns(ctx context.Context, date Date) (*DailyWeighIns, error) {
	var res DailyWeighIns
	if err := s.c.getJSON(ctx, "/weight-service/weight/dayview/"+date.String(), urlValues("includeAll", "true"), &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// DailyWeighIns is the day-view weight payload.
type DailyWeighIns struct {
	StartDate      Date `json:"startDate"`
	EndDate        Date `json:"endDate"`
	DateWeightList []struct {
		SamplePk   int64    `json:"samplePk"`
		Date       int64    `json:"date"`   // ms epoch
		Weight     *float64 `json:"weight"` // grams
		SourceType string   `json:"sourceType"`
	} `json:"dateWeightList"`
	TotalAverage json.RawMessage `json:"totalAverage,omitempty"`
}

// AddWeighIn records a manual weigh-in at t (zero = now).
func (s *WeightService) AddWeighIn(ctx context.Context, weight float64, unit WeightUnit, t time.Time) (json.RawMessage, error) {
	if unit != Kilograms && unit != Pounds {
		return nil, fmt.Errorf("garmin: invalid weight unit %q (want kg or lbs)", unit)
	}
	if t.IsZero() {
		t = time.Now()
	}
	body := map[string]any{
		"dateTimestamp": localTimestamp(t),
		"gmtTimestamp":  gmtTimestamp(t),
		"unitKey":       string(unit),
		"sourceType":    "MANUAL",
		"value":         weight,
	}
	var raw json.RawMessage
	err := s.c.Do(ctx, http.MethodPost, "/weight-service/user-weight", nil, body, &raw)
	return raw, err
}

// DeleteWeighIn removes one weigh-in by sample key and date.
func (s *WeightService) DeleteWeighIn(ctx context.Context, samplePk int64, date Date) error {
	path := fmt.Sprintf("/weight-service/weight/%s/byversion/%d", date, samplePk)
	return s.c.Do(ctx, http.MethodDelete, path, nil, nil, nil)
}

// DeleteAllWeighIns removes every weigh-in of a calendar date and returns how
// many were deleted.
func (s *WeightService) DeleteAllWeighIns(ctx context.Context, date Date) (int, error) {
	day, err := s.DailyWeighIns(ctx, date)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, w := range day.DateWeightList {
		if err := s.DeleteWeighIn(ctx, w.SamplePk, date); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// BodyCompositionEntry is a full body-composition measurement pushed as a
// generated FIT file (Garmin only accepts composition fields through file
// upload). Zero-valued optional fields are omitted.
type BodyCompositionEntry struct {
	Time              time.Time // zero = now
	WeightKG          float64   // required
	PercentFat        float64
	PercentHydration  float64
	VisceralFatMass   float64 // kg
	BoneMass          float64 // kg
	MuscleMass        float64 // kg
	BasalMet          float64 // kcal
	ActiveMet         float64 // kcal
	PhysiqueRating    float64
	MetabolicAge      float64 // years
	VisceralFatRating float64
	BMI               float64
}

// AddBodyComposition uploads a body-composition measurement (as an in-memory
// FIT file, like python-garminconnect does).
func (s *WeightService) AddBodyComposition(ctx context.Context, e BodyCompositionEntry) (*UploadResult, error) {
	if e.WeightKG <= 0 {
		return nil, fmt.Errorf("garmin: weight is required")
	}
	t := e.Time
	if t.IsZero() {
		t = time.Now()
	}
	fit := fitenc.EncodeWeight(fitenc.Weight{
		Time:              t,
		WeightKG:          e.WeightKG,
		PercentFat:        e.PercentFat,
		PercentHydration:  e.PercentHydration,
		VisceralFatMass:   e.VisceralFatMass,
		BoneMass:          e.BoneMass,
		MuscleMass:        e.MuscleMass,
		BasalMet:          e.BasalMet,
		ActiveMet:         e.ActiveMet,
		PhysiqueRating:    e.PhysiqueRating,
		MetabolicAge:      e.MetabolicAge,
		VisceralFatRating: e.VisceralFatRating,
		BMI:               e.BMI,
	})
	return s.c.Upload.Activity(ctx, "body_composition.fit", bytes.NewReader(fit))
}
