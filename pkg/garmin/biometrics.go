package garmin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// BiometricsService accesses biometric-service endpoints (lactate threshold,
// FTP, heart-rate and power zones).
type BiometricsService struct{ c *Client }

var sportKeyRe = regexp.MustCompile(`^[A-Z_]+$`)

// LatestLactateThreshold returns the latest lactate-threshold values plus the
// latest running power-to-weight snapshot.
func (s *BiometricsService) LatestLactateThreshold(ctx context.Context) (latest, powerToWeight json.RawMessage, err error) {
	if err = s.c.getJSON(ctx, "/biometric-service/biometric/latestLactateThreshold", nil, &latest); err != nil {
		return nil, nil, err
	}
	path := "/biometric-service/biometric/powerToWeight/latest/" + Today().String()
	if err = s.c.getJSON(ctx, path, urlValues("sport", "Running"), &powerToWeight); err != nil {
		return latest, nil, err
	}
	return latest, powerToWeight, nil
}

// LactateThresholdRange returns lactate-threshold speed, heart-rate and FTP
// stats over [start, end] (aggregation daily|weekly|monthly|yearly).
func (s *BiometricsService) LactateThresholdRange(ctx context.Context, start, end Date, aggregation string) (speed, heartRate, power json.RawMessage, err error) {
	switch aggregation {
	case "daily", "weekly", "monthly", "yearly":
	default:
		return nil, nil, nil, fmt.Errorf("garmin: aggregation %q must be daily, weekly, monthly or yearly", aggregation)
	}
	q := url.Values{"sport": {"RUNNING"}, "aggregation": {aggregation}, "aggregationStrategy": {"LATEST"}}
	get := func(metric string) (json.RawMessage, error) {
		var raw json.RawMessage
		path := fmt.Sprintf("/biometric-service/stats/%s/range/%s/%s", metric, start, end)
		err := s.c.getJSON(ctx, path, q, &raw)
		return raw, err
	}
	if speed, err = get("lactateThresholdSpeed"); err != nil {
		return
	}
	if heartRate, err = get("lactateThresholdHeartRate"); err != nil {
		return
	}
	power, err = get("functionalThresholdPower")
	return
}

// CyclingFTP returns the latest cycling functional threshold power.
func (s *BiometricsService) CyclingFTP(ctx context.Context) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.c.getJSON(ctx, "/biometric-service/biometric/latestFunctionalThresholdPower/CYCLING", nil, &raw)
	return raw, err
}

// HeartRateZones returns the configured heart-rate zones.
func (s *BiometricsService) HeartRateZones(ctx context.Context) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.c.getJSON(ctx, "/biometric-service/heartRateZones", nil, &raw)
	return raw, err
}

// UpdateHeartRateZones updates the heart-rate zone floors of one sport's zone
// configuration ("DEFAULT" is the account-wide one). It reads the current
// configurations, patches the matching entry — zone floors, plus the stored
// max heart rate and training method when provided (maxHR 0 and
// trainingMethod "" leave them untouched) — marks it CHANGED and PUTs the
// full list back, so fields this library does not model are preserved.
// Returns the configurations as re-read after the update.
func (s *BiometricsService) UpdateHeartRateZones(ctx context.Context, sport string, floors [5]int, maxHR int, trainingMethod string) (json.RawMessage, error) {
	key := strings.ToUpper(strings.TrimSpace(sport))
	if key == "" {
		key = "DEFAULT"
	}
	if !sportKeyRe.MatchString(key) {
		return nil, fmt.Errorf("garmin: invalid sport key %q", sport)
	}
	for i := 1; i < len(floors); i++ {
		if floors[i] <= floors[i-1] {
			return nil, fmt.Errorf("garmin: zone floors must be strictly increasing (zone%dFloor %d <= zone%dFloor %d)", i+1, floors[i], i, floors[i-1])
		}
	}
	var configs []map[string]any
	if err := s.c.getJSON(ctx, "/biometric-service/heartRateZones", nil, &configs); err != nil {
		return nil, err
	}
	patched := false
	for _, cfg := range configs {
		if sportOf, _ := cfg["sport"].(string); sportOf != key {
			continue
		}
		for i, f := range floors {
			cfg[fmt.Sprintf("zone%dFloor", i+1)] = f
		}
		if maxHR > 0 {
			cfg["maxHeartRateUsed"] = maxHR
		}
		if trainingMethod != "" {
			cfg["trainingMethod"] = trainingMethod
		}
		cfg["changeState"] = "CHANGED"
		patched = true
	}
	if !patched {
		return nil, fmt.Errorf("garmin: no heart-rate zone configuration for sport %q", key)
	}
	if err := s.c.Do(ctx, http.MethodPut, "/biometric-service/heartRateZones", nil, configs, nil); err != nil {
		return nil, err
	}
	return s.HeartRateZones(ctx)
}

// PowerZones returns the configured power zones for all sports.
func (s *BiometricsService) PowerZones(ctx context.Context) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.c.getJSON(ctx, "/biometric-service/powerZones/sports/all", nil, &raw)
	return raw, err
}

// PowerZonesForSport returns the power zones of one sport (e.g. "CYCLING";
// lowercase input is normalized).
func (s *BiometricsService) PowerZonesForSport(ctx context.Context, sport string) (json.RawMessage, error) {
	key := strings.ToUpper(strings.TrimSpace(sport))
	if !sportKeyRe.MatchString(key) {
		return nil, fmt.Errorf("garmin: invalid sport key %q", sport)
	}
	var raw json.RawMessage
	err := s.c.getJSON(ctx, "/biometric-service/powerZones/sport/"+key, nil, &raw)
	return raw, err
}
