package garmin

import (
	"context"
	"encoding/json"
	"iter"
	"net/url"
	"strconv"
)

// RecordsService accesses personal records, badges and challenges.
type RecordsService struct{ c *Client }

// PersonalRecords returns the account's personal records.
func (s *RecordsService) PersonalRecords(ctx context.Context) (json.RawMessage, error) {
	dn, err := s.c.DisplayName(ctx)
	if err != nil {
		return nil, err
	}
	var raw json.RawMessage
	err = s.c.getJSON(ctx, "/personalrecord-service/personalrecord/prs/"+dn, nil, &raw)
	return raw, err
}

// Badge is one earned/available badge (essential fields).
type Badge struct {
	BadgeID            int64    `json:"badgeId"`
	BadgeKey           string   `json:"badgeKey"`
	BadgeName          string   `json:"badgeName"`
	BadgePoints        *float64 `json:"badgePoints"`
	BadgeEarnedDate    string   `json:"badgeEarnedDate,omitempty"`
	BadgeEarnedNumber  *int     `json:"badgeEarnedNumber"`
	BadgeProgressValue *float64 `json:"badgeProgressValue"`
	BadgeTargetValue   *float64 `json:"badgeTargetValue"`
	BadgeLimitCount    *int     `json:"badgeLimitCount"`
}

// EarnedBadges returns the badges the account has earned.
func (s *RecordsService) EarnedBadges(ctx context.Context) ([]Badge, error) {
	var badges []Badge
	err := s.c.getJSON(ctx, "/badge-service/badge/earned", nil, &badges)
	return badges, err
}

// AvailableBadges returns the badges still available.
func (s *RecordsService) AvailableBadges(ctx context.Context) ([]Badge, error) {
	var badges []Badge
	err := s.c.getJSON(ctx, "/badge-service/badge/available", urlValues("showExclusiveBadge", "true"), &badges)
	return badges, err
}

// InProgressBadges returns badges with partial progress, deduplicated by id
// (client-side filter over earned + available, like python-garminconnect).
func (s *RecordsService) InProgressBadges(ctx context.Context) ([]Badge, error) {
	earned, err := s.EarnedBadges(ctx)
	if err != nil {
		return nil, err
	}
	available, err := s.AvailableBadges(ctx)
	if err != nil {
		return nil, err
	}
	seen := map[int64]bool{}
	var out []Badge
	for _, b := range append(earned, available...) {
		if seen[b.BadgeID] {
			continue
		}
		progressing := b.BadgeProgressValue != nil && *b.BadgeProgressValue > 0 &&
			b.BadgeTargetValue != nil && *b.BadgeProgressValue < *b.BadgeTargetValue
		repeatable := b.BadgeLimitCount != nil && b.BadgeEarnedNumber != nil &&
			*b.BadgeEarnedNumber > 0 && *b.BadgeEarnedNumber < *b.BadgeLimitCount
		if progressing || repeatable {
			seen[b.BadgeID] = true
			out = append(out, b)
		}
	}
	return out, nil
}

func (s *RecordsService) pagedRaw(ctx context.Context, path string, opts *ListOptions, minStart int) (json.RawMessage, error) {
	start, limit := opts.startLimit(100)
	if start < minStart {
		start = minStart
	}
	q := url.Values{"start": {strconv.Itoa(start)}, "limit": {strconv.Itoa(limit)}}
	var raw json.RawMessage
	err := s.c.getJSON(ctx, path, q, &raw)
	return raw, err
}

// AdhocChallenges returns historical ad-hoc challenges.
func (s *RecordsService) AdhocChallenges(ctx context.Context, opts *ListOptions) (json.RawMessage, error) {
	return s.pagedRaw(ctx, "/adhocchallenge-service/adHocChallenge/historical", opts, 0)
}

// CompletedBadgeChallenges returns completed badge challenges.
func (s *RecordsService) CompletedBadgeChallenges(ctx context.Context, opts *ListOptions) (json.RawMessage, error) {
	return s.pagedRaw(ctx, "/badgechallenge-service/badgeChallenge/completed", opts, 0)
}

// AvailableBadgeChallenges returns available badge challenges.
func (s *RecordsService) AvailableBadgeChallenges(ctx context.Context, opts *ListOptions) (json.RawMessage, error) {
	return s.pagedRaw(ctx, "/badgechallenge-service/badgeChallenge/available", opts, 0)
}

// NonCompletedBadgeChallenges returns badge challenges not yet completed.
func (s *RecordsService) NonCompletedBadgeChallenges(ctx context.Context, opts *ListOptions) (json.RawMessage, error) {
	return s.pagedRaw(ctx, "/badgechallenge-service/badgeChallenge/non-completed", opts, 0)
}

// InProgressVirtualChallenges returns in-progress virtual challenges (Garmin
// requires start > 0 here).
func (s *RecordsService) InProgressVirtualChallenges(ctx context.Context, opts *ListOptions) (json.RawMessage, error) {
	return s.pagedRaw(ctx, "/badgechallenge-service/virtualChallenge/inProgress", opts, 1)
}

// GoalsService accesses goal-service endpoints.
type GoalsService struct{ c *Client }

// GoalStatus filters Goals listings.
type GoalStatus string

const (
	GoalsActive GoalStatus = "active"
	GoalsFuture GoalStatus = "future"
	GoalsPast   GoalStatus = "past"
)

// List returns one page of goals with the given status.
func (s *GoalsService) List(ctx context.Context, status GoalStatus, opts *ListOptions) ([]json.RawMessage, error) {
	start, limit := opts.startLimit(30)
	q := url.Values{
		"status":    {string(status)},
		"start":     {strconv.Itoa(start)},
		"limit":     {strconv.Itoa(limit)},
		"sortOrder": {"asc"},
	}
	var goals []json.RawMessage
	err := s.c.getJSON(ctx, "/goal-service/goal/goals", q, &goals)
	return goals, err
}

// All iterates over every goal with the given status, paginating until an
// empty page.
func (s *GoalsService) All(ctx context.Context, status GoalStatus) iter.Seq2[json.RawMessage, error] {
	return paged(30, func(start, limit int) ([]json.RawMessage, error) {
		return s.List(ctx, status, &ListOptions{Start: start, Limit: limit})
	})
}
