package garmin

import (
	"context"
	"encoding/json"
)

// UserProfileService accesses userprofile-service endpoints.
type UserProfileService struct{ c *Client }

// SocialProfile is the Garmin Connect social profile.
type SocialProfile struct {
	ID                    int64  `json:"id"`
	ProfileID             int64  `json:"profileId"`
	DisplayName           string `json:"displayName"` // UUID-like identifier used in API paths
	FullName              string `json:"fullName"`
	UserName              string `json:"userName"`
	ProfileImageURLLarge  string `json:"profileImageUrlLarge"`
	ProfileImageURLMedium string `json:"profileImageUrlMedium"`
	ProfileImageURLSmall  string `json:"profileImageUrlSmall"`
	Location              string `json:"location"`
	UserLevel             int    `json:"userLevel"`
	UserPoint             int    `json:"userPoint"`
}

// SocialProfile returns the account's social profile (also the cheapest way
// to verify that credentials work).
func (s *UserProfileService) SocialProfile(ctx context.Context) (*SocialProfile, error) {
	var p SocialProfile
	if err := s.c.getJSON(ctx, "/userprofile-service/socialProfile", nil, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// UserSettings is the user-settings payload; only commonly-used fields are
// typed, the full payload stays available in Raw.
type UserSettings struct {
	ID       int64 `json:"id"`
	UserData struct {
		Gender                    string   `json:"gender"`
		Weight                    *float64 `json:"weight"` // grams
		Height                    *float64 `json:"height"` // centimeters
		TimeFormat                string   `json:"timeFormat"`
		BirthDate                 string   `json:"birthDate"`
		MeasurementSystem         string   `json:"measurementSystem"` // "metric" | "statute_us" | ...
		Handedness                string   `json:"handedness"`
		VO2MaxRunning             *float64 `json:"vo2MaxRunning"`
		VO2MaxCycling             *float64 `json:"vo2MaxCycling"`
		LactateThresholdSpeed     *float64 `json:"lactateThresholdSpeed"`
		LactateThresholdHeartRate *float64 `json:"lactateThresholdHeartRate"`
	} `json:"userData"`
	UserSleep struct {
		SleepTime        int  `json:"sleepTime"` // seconds from midnight
		WakeTime         int  `json:"wakeTime"`
		DefaultSleepTime bool `json:"defaultSleepTime"`
		DefaultWakeTime  bool `json:"defaultWakeTime"`
	} `json:"userSleep"`
	Raw json.RawMessage `json:"-"`
}

// Settings returns the account user settings (measurement system, biometric
// baselines, sleep window…).
func (s *UserProfileService) Settings(ctx context.Context) (*UserSettings, error) {
	var raw json.RawMessage
	if err := s.c.getJSON(ctx, "/userprofile-service/userprofile/user-settings", nil, &raw); err != nil {
		return nil, err
	}
	var us UserSettings
	if err := json.Unmarshal(raw, &us); err != nil {
		return nil, err
	}
	us.Raw = raw
	return &us, nil
}

// ProfileSettings returns the profile-level settings payload
// (/userprofile-service/userprofile/settings) as raw JSON.
func (s *UserProfileService) ProfileSettings(ctx context.Context) (json.RawMessage, error) {
	var raw json.RawMessage
	if err := s.c.getJSON(ctx, "/userprofile-service/userprofile/settings", nil, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}
