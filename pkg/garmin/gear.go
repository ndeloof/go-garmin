package garmin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// GearService accesses gear-service endpoints (shoes, bikes…).
type GearService struct{ c *Client }

// Gear is one piece of gear (essential fields).
type Gear struct {
	GearPk          int64    `json:"gearPk"`
	UUID            string   `json:"uuid"`
	UserProfilePk   int64    `json:"userProfilePk"`
	GearMakeName    string   `json:"gearMakeName"`
	GearModelName   string   `json:"gearModelName"`
	DisplayName     string   `json:"displayName"`
	CustomMakeModel string   `json:"customMakeModel"`
	GearStatusName  string   `json:"gearStatusName"`
	DateBegin       string   `json:"dateBegin,omitempty"`
	DateEnd         string   `json:"dateEnd,omitempty"`
	MaximumMeters   *float64 `json:"maximumMeters,omitempty"`
}

// List returns the gear of a user profile (SocialProfile.ProfileID).
func (s *GearService) List(ctx context.Context, userProfilePk int64) ([]Gear, error) {
	q := url.Values{"userProfilePk": {strconv.FormatInt(userProfilePk, 10)}}
	var gear []Gear
	err := s.c.getJSON(ctx, "/gear-service/gear/filterGear", q, &gear)
	return gear, err
}

// ForActivity returns the gear linked to an activity.
func (s *GearService) ForActivity(ctx context.Context, activityID int64) ([]Gear, error) {
	q := url.Values{"activityId": {strconv.FormatInt(activityID, 10)}}
	var gear []Gear
	err := s.c.getJSON(ctx, "/gear-service/gear/filterGear", q, &gear)
	return gear, err
}

// Stats returns usage stats of one piece of gear. Retired/unknown gear (404)
// returns an empty payload, not an error.
func (s *GearService) Stats(ctx context.Context, gearUUID string) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.c.getJSON(ctx, "/gear-service/gear/stats/"+gearUUID, nil, &raw)
	if errors.Is(err, ErrNotFound) {
		return json.RawMessage("{}"), nil
	}
	return raw, err
}

// Activities returns the activities linked to a piece of gear. Unknown gear
// (404) returns an empty list.
func (s *GearService) Activities(ctx context.Context, gearUUID string, opts *ListOptions) ([]Activity, error) {
	start, limit := opts.startLimit(1000)
	q := url.Values{"start": {strconv.Itoa(start)}, "limit": {strconv.Itoa(limit)}}
	var raw json.RawMessage
	err := s.c.getJSON(ctx, fmt.Sprintf("/activitylist-service/activities/%s/gear", gearUUID), q, &raw)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return decodeActivityList(raw)
}

// Defaults returns the default gear per activity type.
func (s *GearService) Defaults(ctx context.Context, userProfilePk int64) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.c.getJSON(ctx, fmt.Sprintf("/gear-service/gear/user/%d/activityTypes", userProfilePk), nil, &raw)
	return raw, err
}

// SetDefault makes (or unmakes) a piece of gear the default for an activity
// type id.
func (s *GearService) SetDefault(ctx context.Context, gearUUID string, activityTypeID int64, isDefault bool) error {
	if isDefault {
		path := fmt.Sprintf("/gear-service/gear/%s/activityType/%d/default/true", gearUUID, activityTypeID)
		return s.c.Do(ctx, http.MethodPut, path, nil, nil, nil)
	}
	path := fmt.Sprintf("/gear-service/gear/%s/activityType/%d", gearUUID, activityTypeID)
	return s.c.Do(ctx, http.MethodDelete, path, nil, nil, nil)
}

// LinkToActivity links a piece of gear to an activity.
func (s *GearService) LinkToActivity(ctx context.Context, gearUUID string, activityID int64) error {
	path := fmt.Sprintf("/gear-service/gear/link/%s/activity/%d", gearUUID, activityID)
	return wrapGearNotFound(s.c.Do(ctx, http.MethodPut, path, nil, nil, nil), gearUUID, activityID)
}

// UnlinkFromActivity unlinks a piece of gear from an activity.
func (s *GearService) UnlinkFromActivity(ctx context.Context, gearUUID string, activityID int64) error {
	path := fmt.Sprintf("/gear-service/gear/unlink/%s/activity/%d", gearUUID, activityID)
	return wrapGearNotFound(s.c.Do(ctx, http.MethodPut, path, nil, nil, nil), gearUUID, activityID)
}

func wrapGearNotFound(err error, gearUUID string, activityID int64) error {
	if errors.Is(err, ErrNotFound) {
		return fmt.Errorf("gear %s or activity %d not found: %w", gearUUID, activityID, err)
	}
	return err
}
