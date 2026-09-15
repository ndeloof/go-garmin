package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

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
func numProp(desc string) map[string]any {
	return map[string]any{"type": "number", "description": desc}
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
func dateTool(name, desc string, fn func(context.Context, garmin.Date) (any, error)) Tool {
	return Tool{
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

func rangeTool(name, desc string, defDays int, fn func(context.Context, garmin.Date, garmin.Date) (any, error)) Tool {
	return Tool{
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

func noArgTool(name, desc string, fn func(context.Context) (any, error)) Tool {
	return Tool{
		Name:        name,
		Description: desc,
		Handler:     func(ctx context.Context, _ json.RawMessage) (any, error) { return fn(ctx) },
	}
}

// garminTools returns the full tool set exposed for a client.
func garminTools(c *garmin.Client) []Tool {
	return []Tool{
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
		{
			Name:        "create_activity",
			Description: "Create a manual activity (no device file): name, type key, local start time, duration, distance, and optionally an elevation gain (applied as a summary patch right after creation). Returns the created activity.",
			Schema: objectSchema(map[string]any{
				"name":             strProp("Activity name"),
				"type_key":         strProp("Activity type key, e.g. walking, hiking, running"),
				"start_local":      strProp("Local wall-clock start, YYYY-MM-DDTHH:MM:SS"),
				"timezone":         strProp("IANA timezone unit key (default: Europe/Paris)"),
				"duration_sec":     numProp("Duration in seconds"),
				"distance_m":       numProp("Distance in meters"),
				"elevation_gain_m": numProp("Total ascent in meters (optional)"),
			}, "name", "type_key", "start_local", "duration_sec", "distance_m"),
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				a, err := decode[struct {
					Name        string  `json:"name"`
					TypeKey     string  `json:"type_key"`
					StartLocal  string  `json:"start_local"`
					Timezone    string  `json:"timezone"`
					DurationSec float64 `json:"duration_sec"`
					DistanceM   float64 `json:"distance_m"`
					ElevGainM   float64 `json:"elevation_gain_m"`
				}](raw)
				if err != nil {
					return nil, err
				}
				start, err := time.Parse("2006-01-02T15:04:05", a.StartLocal)
				if err != nil {
					return nil, fmt.Errorf("invalid start_local (want YYYY-MM-DDTHH:MM:SS): %w", err)
				}
				tz := a.Timezone
				if tz == "" {
					tz = "Europe/Paris"
				}
				created, err := c.Activities.CreateManual(ctx, garmin.ManualActivity{
					Name: a.Name, TypeKey: a.TypeKey, Start: start, TimeZone: tz,
					Distance: a.DistanceM, Duration: a.DurationSec,
				})
				if err != nil {
					return nil, err
				}
				if a.ElevGainM > 0 {
					if err := c.Activities.UpdateSummary(ctx, created.ActivityID, map[string]any{"elevationGain": a.ElevGainM}); err != nil {
						return nil, fmt.Errorf("activity %d created but elevation update failed: %w", created.ActivityID, err)
					}
				}
				return c.Activities.Get(ctx, created.ActivityID)
			},
		},
		{
			Name:        "update_activity",
			Description: "Edit an activity: rename it, change its description or type, and/or patch summary fields (distance, duration, elevation gain/loss) the way the Connect web editor does. Summary fields are only honored on activities without sensor data for them (manual or indoor activities). Returns the refreshed activity summary.",
			Schema: objectSchema(map[string]any{
				"activity_id":      intProp("Garmin activity id"),
				"name":             strProp("New activity name (omit: unchanged)"),
				"description":      strProp("New description (omit: unchanged)"),
				"type_key":         strProp("New activity type key, e.g. walking, hiking, treadmill_running (omit: unchanged)"),
				"distance_m":       numProp("Distance in meters (omit: unchanged)"),
				"duration_sec":     numProp("Duration in seconds (omit: unchanged)"),
				"elevation_gain_m": numProp("Total ascent in meters (omit: unchanged)"),
				"elevation_loss_m": numProp("Total descent in meters (omit: unchanged)"),
			}, "activity_id"),
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				a, err := decode[struct {
					ActivityID  int64    `json:"activity_id"`
					Name        string   `json:"name"`
					Description *string  `json:"description"`
					TypeKey     string   `json:"type_key"`
					DistanceM   *float64 `json:"distance_m"`
					DurationSec *float64 `json:"duration_sec"`
					ElevGainM   *float64 `json:"elevation_gain_m"`
					ElevLossM   *float64 `json:"elevation_loss_m"`
				}](raw)
				if err != nil {
					return nil, err
				}
				if a.ActivityID == 0 {
					return nil, fmt.Errorf("activity_id is required")
				}
				if a.Name != "" {
					if err := c.Activities.SetName(ctx, a.ActivityID, a.Name); err != nil {
						return nil, err
					}
				}
				if a.Description != nil {
					if err := c.Activities.SetDescription(ctx, a.ActivityID, *a.Description); err != nil {
						return nil, err
					}
				}
				if a.TypeKey != "" {
					if err := c.Activities.SetType(ctx, a.ActivityID, garmin.ActivityType{TypeKey: a.TypeKey}); err != nil {
						return nil, err
					}
				}
				summary := map[string]any{}
				if a.DistanceM != nil {
					summary["distance"] = *a.DistanceM
				}
				if a.DurationSec != nil {
					summary["duration"] = *a.DurationSec
				}
				if a.ElevGainM != nil {
					summary["elevationGain"] = *a.ElevGainM
				}
				if a.ElevLossM != nil {
					summary["elevationLoss"] = *a.ElevLossM
				}
				if len(summary) > 0 {
					if err := c.Activities.UpdateSummary(ctx, a.ActivityID, summary); err != nil {
						return nil, err
					}
				}
				return c.Activities.Get(ctx, a.ActivityID)
			},
		},

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
		{
			Name:        "set_hr_zones",
			Description: "Update the configured heart-rate zone floors (lower bpm bound of each zone, strictly increasing) for one sport's configuration. Optionally also stores a new max heart rate and training method. Returns the configurations as re-read after the update.",
			Schema: objectSchema(map[string]any{
				"zone1_floor":     intProp("Zone 1 lower bound, bpm"),
				"zone2_floor":     intProp("Zone 2 lower bound, bpm"),
				"zone3_floor":     intProp("Zone 3 lower bound, bpm"),
				"zone4_floor":     intProp("Zone 4 lower bound, bpm"),
				"zone5_floor":     intProp("Zone 5 lower bound, bpm"),
				"max_hr":          intProp("Max heart rate to store (omit: unchanged)"),
				"sport":           strProp("Sport configuration to update (default: DEFAULT, the account-wide one)"),
				"training_method": strProp("Training method key to store, e.g. HR_MAX (omit: unchanged)"),
			}, "zone1_floor", "zone2_floor", "zone3_floor", "zone4_floor", "zone5_floor"),
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				a, err := decode[struct {
					Z1             int    `json:"zone1_floor"`
					Z2             int    `json:"zone2_floor"`
					Z3             int    `json:"zone3_floor"`
					Z4             int    `json:"zone4_floor"`
					Z5             int    `json:"zone5_floor"`
					MaxHR          int    `json:"max_hr"`
					Sport          string `json:"sport"`
					TrainingMethod string `json:"training_method"`
				}](raw)
				if err != nil {
					return nil, err
				}
				floors := [5]int{a.Z1, a.Z2, a.Z3, a.Z4, a.Z5}
				return c.Biometrics.UpdateHeartRateZones(ctx, a.Sport, floors, a.MaxHR, a.TrainingMethod)
			},
		},

		// Weight & records.
		rangeTool("get_weight", "Get weight and body-composition data over a range (default last 30 days).", 30,
			func(ctx context.Context, s, e garmin.Date) (any, error) { return c.Weight.BodyComposition(ctx, s, e) }),
		noArgTool("get_personal_records", "Get the account's personal records.",
			func(ctx context.Context) (any, error) { return c.Records.PersonalRecords(ctx) }),

		// Workouts.
		{
			Name:        "list_workouts",
			Description: "List the account's structured workouts.",
			Schema: objectSchema(map[string]any{
				"limit": intProp("Max workouts to return (default 100)"),
				"start": intProp("Offset for pagination (default 0)"),
			}),
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				a, err := decode[struct {
					Limit int `json:"limit"`
					Start int `json:"start"`
				}](raw)
				if err != nil {
					return nil, err
				}
				return c.Workouts.List(ctx, &garmin.ListOptions{Start: a.Start, Limit: a.Limit})
			},
		},
		workoutIDTool("get_workout", "Get one structured workout's full payload.",
			func(ctx context.Context, id int64) (any, error) { return c.Workouts.Get(ctx, id) }),
		{
			Name:        "create_workout",
			Description: "Create a structured workout. Takes the full workout-service JSON payload (workoutName, sportType, workoutSegments with ExecutableStepDTO/RepeatGroupDTO steps — the same shape returned by get_workout).",
			Schema: objectSchema(map[string]any{
				"workout": map[string]any{"type": "object", "description": "Full workout-service workout payload"},
			}, "workout"),
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				a, err := decode[struct {
					Workout json.RawMessage `json:"workout"`
				}](raw)
				if err != nil {
					return nil, err
				}
				if len(a.Workout) == 0 {
					return nil, fmt.Errorf("workout is required")
				}
				return c.Workouts.Create(ctx, a.Workout)
			},
		},
		workoutIDTool("delete_workout", "Delete a structured workout.",
			func(ctx context.Context, id int64) (any, error) { return nil, c.Workouts.Delete(ctx, id) }),
		{
			Name:        "schedule_workout",
			Description: "Schedule a workout on the Garmin Connect calendar.",
			Schema: objectSchema(map[string]any{
				"workout_id": intProp("Garmin workout id"),
				"date":       dateProp,
			}, "workout_id"),
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				a, err := decode[struct {
					WorkoutID int64  `json:"workout_id"`
					Date      string `json:"date"`
				}](raw)
				if err != nil {
					return nil, err
				}
				if a.WorkoutID == 0 {
					return nil, fmt.Errorf("workout_id is required")
				}
				d, err := dateArgs{Date: a.Date}.date()
				if err != nil {
					return nil, err
				}
				return c.Workouts.Schedule(ctx, a.WorkoutID, d)
			},
		},

		// Courses (GPS routes).
		noArgTool("list_courses", "List the account's courses (GPS routes).",
			func(ctx context.Context) (any, error) { return c.Courses.List(ctx) }),
		courseIDTool("get_course", "Get one course's full payload (metadata + geo points).",
			func(ctx context.Context, id int64) (any, error) { return c.Courses.Get(ctx, id) }),
		courseIDTool("delete_course", "Delete a course. A freshly imported course may answer HTTP 429 while still processing — retry after a few seconds.",
			func(ctx context.Context, id int64) (any, error) { return nil, c.Courses.Delete(ctx, id) }),
		{
			Name:        "import_course_gpx",
			Description: "Create a course from GPX content (parse + save). Returns the created course with its courseId.",
			Schema: objectSchema(map[string]any{
				"gpx":         strProp("GPX file content (XML)"),
				"course_name": strProp("Course name (default: the GPX track name)"),
			}, "gpx"),
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				a, err := decode[struct {
					GPX        string `json:"gpx"`
					CourseName string `json:"course_name"`
				}](raw)
				if err != nil {
					return nil, err
				}
				if a.GPX == "" {
					return nil, fmt.Errorf("gpx is required")
				}
				return c.Courses.ImportGPX(ctx, "import.gpx", strings.NewReader(a.GPX), a.CourseName)
			},
		},
		{
			Name:        "push_course_to_device",
			Description: "Queue a course for delivery to a device (synced on the device's next connection). Use get_devices to find device ids.",
			Schema: objectSchema(map[string]any{
				"course_id": intProp("Garmin course id"),
				"device_id": intProp("Garmin device id"),
			}, "course_id", "device_id"),
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				a, err := decode[struct {
					CourseID int64 `json:"course_id"`
					DeviceID int64 `json:"device_id"`
				}](raw)
				if err != nil {
					return nil, err
				}
				if a.CourseID == 0 || a.DeviceID == 0 {
					return nil, fmt.Errorf("course_id and device_id are required")
				}
				return c.Courses.PushToDevice(ctx, a.CourseID, a.DeviceID)
			},
		},
	}
}

// courseIDTool wires a tool taking a required course_id.
func courseIDTool(name, desc string, fn func(context.Context, int64) (any, error)) Tool {
	return Tool{
		Name:        name,
		Description: desc,
		Schema:      objectSchema(map[string]any{"course_id": intProp("Garmin course id")}, "course_id"),
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			a, err := decode[struct {
				CourseID int64 `json:"course_id"`
			}](raw)
			if err != nil {
				return nil, err
			}
			if a.CourseID == 0 {
				return nil, fmt.Errorf("course_id is required")
			}
			return fn(ctx, a.CourseID)
		},
	}
}

// workoutIDTool wires a tool taking a required workout_id.
func workoutIDTool(name, desc string, fn func(context.Context, int64) (any, error)) Tool {
	return Tool{
		Name:        name,
		Description: desc,
		Schema:      objectSchema(map[string]any{"workout_id": intProp("Garmin workout id")}, "workout_id"),
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			a, err := decode[struct {
				WorkoutID int64 `json:"workout_id"`
			}](raw)
			if err != nil {
				return nil, err
			}
			if a.WorkoutID == 0 {
				return nil, fmt.Errorf("workout_id is required")
			}
			return fn(ctx, a.WorkoutID)
		},
	}
}

// activityIDTool wires a tool taking a required activity_id.
func activityIDTool(name, desc string, fn func(context.Context, int64) (any, error)) Tool {
	return Tool{
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
