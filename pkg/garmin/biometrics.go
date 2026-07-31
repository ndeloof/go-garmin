package garmin

import (
	"context"
	"encoding/json"
	"fmt"
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
