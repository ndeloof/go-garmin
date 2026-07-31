package garmin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// GolfService accesses gcs-golfcommunity endpoints. Note: these endpoints use
// dash-separated query parameters (per-page, scorecard-ids…).
type GolfService struct{ c *Client }

// Summary returns golf scorecard summaries.
func (s *GolfService) Summary(ctx context.Context, opts *ListOptions) (json.RawMessage, error) {
	start, limit := opts.startLimit(100)
	q := url.Values{"per-page": {strconv.Itoa(limit)}, "start": {strconv.Itoa(start)}}
	var raw json.RawMessage
	err := s.c.getJSON(ctx, "/gcs-golfcommunity/api/v2/scorecard/summary", q, &raw)
	return raw, err
}

// Scorecard returns the detail of one scorecard.
func (s *GolfService) Scorecard(ctx context.Context, scorecardID int64) (json.RawMessage, error) {
	q := url.Values{
		"scorecard-ids":                 {strconv.FormatInt(scorecardID, 10)},
		"include-longest-shot-distance": {"true"},
	}
	var raw json.RawMessage
	err := s.c.getJSON(ctx, "/gcs-golfcommunity/api/v2/scorecard/detail", q, &raw)
	return raw, err
}

var holeNumbersRe = regexp.MustCompile(`^([1-9]|1[0-8])([,-]([1-9]|1[0-8]))*$`)

// ShotData returns per-hole shot data of a scorecard. holeNumbers is an
// optional selection like "1,2,3" or "1-3"; commas are converted to dashes
// (Garmin rejects commas with a 400).
func (s *GolfService) ShotData(ctx context.Context, scorecardID int64, holeNumbers string) (json.RawMessage, error) {
	q := url.Values{}
	if holeNumbers != "" {
		if !holeNumbersRe.MatchString(holeNumbers) {
			return nil, fmt.Errorf("garmin: invalid hole numbers %q", holeNumbers)
		}
		q.Set("hole-numbers", strings.ReplaceAll(holeNumbers, ",", "-"))
	}
	path := fmt.Sprintf("/gcs-golfcommunity/api/v2/shot/scorecard/%d/hole", scorecardID)
	var raw json.RawMessage
	err := s.c.getJSON(ctx, path, q, &raw)
	return raw, err
}

// ClubStats returns per-club statistics.
func (s *GolfService) ClubStats(ctx context.Context, limit int) (json.RawMessage, error) {
	if limit <= 0 {
		limit = 1000
	}
	q := url.Values{"per-page": {strconv.Itoa(limit)}, "include-stats": {"true"}}
	var raw json.RawMessage
	err := s.c.getJSON(ctx, "/gcs-golfcommunity/api/v2/club/player", q, &raw)
	return raw, err
}

// UserStats returns the player's golf statistics.
func (s *GolfService) UserStats(ctx context.Context) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.c.getJSON(ctx, "/gcs-golfcommunity/api/v2/player/stats", nil, &raw)
	return raw, err
}
