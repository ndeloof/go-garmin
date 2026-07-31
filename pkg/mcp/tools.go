package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ndeloof/go-garmin/pkg/garmin"
)

// schema helpers -------------------------------------------------------------

func objectSchema(props map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func strProp(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}
func intProp(desc string) map[string]any {
	return map[string]any{"type": "integer", "description": desc}
}

var dateProp = map[string]any{"type": "string", "description": "Calendar date, YYYY-MM-DD (default: today)"}

// arg decoding ---------------------------------------------------------------

func decode[T any](raw json.RawMessage) (T, error) {
	var v T
	if len(raw) == 0 {
		return v, nil
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return v, fmt.Errorf("invalid arguments: %w", err)
	}
	return v, nil
}

type dateArgs struct {
	Date string `json:"date"`
}

func (a dateArgs) date() (garmin.Date, error) {
	if a.Date == "" {
		return garmin.Today(), nil
	}
	return garmin.ParseDate(a.Date)
}

type rangeArgs struct {
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
}

func (a rangeArgs) span(defDays int) (start, end garmin.Date, err error) {
	end = garmin.Today()
	if a.EndDate != "" {
		if end, err = garmin.ParseDate(a.EndDate); err != nil {
			return
		}
	}
	start = end.AddDays(-defDays)
	if a.StartDate != "" {
		start, err = garmin.ParseDate(a.StartDate)
	}
	return
}

// dateTool wires a "date"-only tool to a client method.
func dateTool(name, desc string, fn func(context.Context, garmin.Date) (any, error)) tool {
	return tool{
		Name:        name,
		Description: desc,
		Schema:      objectSchema(map[string]any{"date": dateProp}),
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			a, err := decode[dateArgs](raw)
			if err != nil {
				return nil, err
			}
			d, err := a.date()
			if err != nil {
				return nil, err
			}
			return fn(ctx, d)
		},
	}
}

func rangeTool(name, desc string, defDays int, fn func(context.Context, garmin.Date, garmin.Date) (any, error)) tool {
	return tool{
		Name:        name,
		Description: desc,
		Schema: objectSchema(map[string]any{
			"start_date": strProp("Range start, YYYY-MM-DD"),
			"end_date":   strProp("Range end, YYYY-MM-DD (default: today)"),
		}),
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			a, err := decode[rangeArgs](raw)
			if err != nil {
				return nil, err
			}
			start, end, err := a.span(defDays)
			if err != nil {
				return nil, err
			}
			return fn(ctx, start, end)
		},
	}
}

func noArgTool(name, desc string, fn func(context.Context) (any, error)) tool {
	return tool{
		Name:        name,
		Description: desc,
		Handler:     func(ctx context.Context, _ json.RawMessage) (any, error) { return fn(ctx) },
	}
}

// garminTools returns the full tool set exposed for a client.
func garminTools(c *garmin.Client) []tool {
	return []tool{
		// Profile & devices.
		noArgTool("get_profile", "Get the Garmin Connect social profile (name, display name, level).",
			func(ctx context.Context) (any, error) { return c.UserProfile.SocialProfile(ctx) }),
		noArgTool("get_user_settings", "Get account user settings (units, biometrics, sleep window).",
			func(ctx context.Context) (any, error) { return c.UserProfile.Settings(ctx) }),
		noArgTool("get_devices", "List the registered Garmin devices.",
			func(ctx context.Context) (any, error) { return c.Devices.List(ctx) }),

		// Activities.
		{
			Name:        "list_activities",
			Description: "List recent activities, most recent first.",
			Schema: objectSchema(map[string]any{
				"limit":         intProp("Max activities to return (default 10)"),
				"start":         intProp("Offset for pagination (default 0)"),
				"activity_type": strProp("Filter by type key, e.g. running, cycling"),
			}),
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				a, err := decode[struct {
					Limit        int    `json:"limit"`
					Start        int    `json:"start"`
					ActivityType string `json:"activity_type"`
				}](raw)
				if err != nil {
					return nil, err
				}
				if a.Limit <= 0 {
					a.Limit = 10
				}
				return c.Activities.List(ctx, &garmin.ActivityListOptions{
					ListOptions:  garmin.ListOptions{Start: a.Start, Limit: a.Limit},
					ActivityType: a.ActivityType,
				})
			},
		},
		activityIDTool("get_activity", "Get one activity's full summary.",
			func(ctx context.Context, id int64) (any, error) { return c.Activities.Get(ctx, id) }),
		activityIDTool("get_activity_details", "Get one activity's sample/chart details (GPS, HR, speed…).",
			func(ctx context.Context, id int64) (any, error) { return c.Activities.Details(ctx, id, nil) }),
		activityIDTool("get_activity_splits", "Get one activity's lap splits.",
			func(ctx context.Context, id int64) (any, error) { return c.Activities.Splits(ctx, id) }),
		activityIDTool("get_activity_weather", "Get the weather recorded during an activity.",
			func(ctx context.Context, id int64) (any, error) { return c.Activities.Weather(ctx, id) }),
		activityIDTool("get_activity_hr_zones", "Get time-in-heart-rate-zones for an activity.",
			func(ctx context.Context, id int64) (any, error) { return c.Activities.HRTimeInZones(ctx, id) }),

		// Daily health & wellness.
		dateTool("get_daily_summary", "Get the daily wellness summary (steps, calories, stress, body battery, RHR).",
			func(ctx context.Context, d garmin.Date) (any, error) { return c.Summaries.Daily(ctx, d) }),
		dateTool("get_steps", "Get the intraday steps chart for a day.",
			func(ctx context.Context, d garmin.Date) (any, error) { return c.Summaries.StepsChart(ctx, d) }),
		dateTool("get_heart_rate", "Get the day's heart-rate samples and resting HR.",
			func(ctx context.Context, d garmin.Date) (any, error) { return c.Wellness.HeartRates(ctx, d) }),
		dateTool("get_sleep", "Get the night's sleep data (stages, duration, scores).",
			func(ctx context.Context, d garmin.Date) (any, error) { return c.Wellness.Sleep(ctx, d) }),
		dateTool("get_stress", "Get the day's all-day stress data.",
			func(ctx context.Context, d garmin.Date) (any, error) { return c.Wellness.Stress(ctx, d) }),
		dateTool("get_hrv", "Get the day's heart-rate-variability data.",
			func(ctx context.Context, d garmin.Date) (any, error) { return c.Wellness.HRV(ctx, d) }),
		dateTool("get_spo2", "Get the day's pulse-ox (SpO2) data.",
			func(ctx context.Context, d garmin.Date) (any, error) { return c.Wellness.SpO2(ctx, d) }),
		dateTool("get_respiration", "Get the day's respiration data.",
			func(ctx context.Context, d garmin.Date) (any, error) { return c.Wellness.Respiration(ctx, d) }),
		dateTool("get_hydration", "Get the day's hydration log.",
			func(ctx context.Context, d garmin.Date) (any, error) { return c.Wellness.Hydration(ctx, d) }),
		rangeTool("get_body_battery", "Get body-battery reports over a date range (default last 7 days).", 7,
			func(ctx context.Context, s, e garmin.Date) (any, error) { return c.Wellness.BodyBattery(ctx, s, e) }),

		// Training metrics.
		dateTool("get_training_readiness", "Get the day's training-readiness score.",
			func(ctx context.Context, d garmin.Date) (any, error) { return c.Metrics.TrainingReadiness(ctx, d) }),
		dateTool("get_training_status", "Get the day's aggregated training status.",
			func(ctx context.Context, d garmin.Date) (any, error) { return c.Metrics.TrainingStatus(ctx, d) }),
		dateTool("get_vo2max", "Get the day's VO2max / max-metrics.",
			func(ctx context.Context, d garmin.Date) (any, error) { return c.Metrics.MaxMetrics(ctx, d) }),
		dateTool("get_endurance_score", "Get the day's endurance score.",
			func(ctx context.Context, d garmin.Date) (any, error) { return c.Metrics.EnduranceScore(ctx, d) }),
		dateTool("get_hill_score", "Get the day's hill score.",
			func(ctx context.Context, d garmin.Date) (any, error) { return c.Metrics.HillScore(ctx, d) }),
		dateTool("get_fitness_age", "Get the day's fitness-age data.",
			func(ctx context.Context, d garmin.Date) (any, error) { return c.Metrics.FitnessAge(ctx, d) }),
		noArgTool("get_race_predictions", "Get the latest race-time predictions.",
			func(ctx context.Context) (any, error) { return c.Metrics.RacePredictions(ctx) }),
		noArgTool("get_hr_zones", "Get the configured heart-rate zones.",
			func(ctx context.Context) (any, error) { return c.Biometrics.HeartRateZones(ctx) }),

		// Weight & records.
		rangeTool("get_weight", "Get weight and body-composition data over a range (default last 30 days).", 30,
			func(ctx context.Context, s, e garmin.Date) (any, error) { return c.Weight.BodyComposition(ctx, s, e) }),
		noArgTool("get_personal_records", "Get the account's personal records.",
			func(ctx context.Context) (any, error) { return c.Records.PersonalRecords(ctx) }),
	}
}

// activityIDTool wires a tool taking a required activity_id.
func activityIDTool(name, desc string, fn func(context.Context, int64) (any, error)) tool {
	return tool{
		Name:        name,
		Description: desc,
		Schema:      objectSchema(map[string]any{"activity_id": intProp("Garmin activity id")}, "activity_id"),
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			a, err := decode[struct {
				ActivityID int64 `json:"activity_id"`
			}](raw)
			if err != nil {
				return nil, err
			}
			if a.ActivityID == 0 {
				return nil, fmt.Errorf("activity_id is required")
			}
			return fn(ctx, a.ActivityID)
		},
	}
}
