//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/ndeloof/go-garmin/pkg/garmin"
)

func TestActivitiesListAndDetails(t *testing.T) {
	c := testClient(t)
	ctx := testCtx(t)

	count, err := c.Activities.Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	t.Logf("account has %d activities", count)

	acts, err := c.Activities.List(ctx, &garmin.ActivityListOptions{
		ListOptions: garmin.ListOptions{Limit: 5},
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if count > 0 && len(acts) == 0 {
		t.Fatal("no activities returned despite non-zero count")
	}
	if len(acts) == 0 {
		t.Skip("account has no activities")
	}

	a := acts[0]
	if a.ActivityID == 0 {
		t.Fatalf("first activity has no id: %+v", a)
	}
	t.Logf("latest: #%d %q type=%s distance=%.0fm", a.ActivityID, a.ActivityName, a.ActivityType.TypeKey, a.Distance)

	got, err := c.Activities.Get(ctx, a.ActivityID)
	if err != nil {
		t.Fatalf("Get(%d): %v", a.ActivityID, err)
	}
	if got.ActivityID != a.ActivityID {
		t.Fatalf("Get returned id %d", got.ActivityID)
	}

	details, err := c.Activities.Details(ctx, a.ActivityID, nil)
	if err != nil {
		t.Fatalf("Details(%d): %v", a.ActivityID, err)
	}
	ts := details.Series(garmin.MetricTimestamp)
	t.Logf("details: %d samples, %d descriptors", len(ts), len(details.MetricDescriptors))
}

func TestActivityDownload(t *testing.T) {
	c := testClient(t)
	ctx := testCtx(t)
	last, err := c.Activities.Last(ctx)
	if errors.Is(err, garmin.ErrNotFound) {
		t.Skip("no activities")
	}
	if err != nil {
		t.Fatalf("Last: %v", err)
	}
	gpx, err := c.Download.ExportActivity(ctx, last.ActivityID, garmin.ExportGPX)
	if err != nil {
		t.Fatalf("ExportActivity GPX: %v", err)
	}
	if len(gpx) == 0 {
		t.Fatal("empty GPX export")
	}
	t.Logf("GPX export: %d bytes", len(gpx))
}

// TestManualStrengthActivityWithExerciseSets exercises the manual-logging
// path this package added on top of the undocumented exerciseSets endpoint:
// create a manual strength activity, attach a typed exercise-set list, and
// verify Garmin echoes back the exact set count and values. The activity is
// always deleted afterward, regardless of outcome, so the account stays
// clean of test data.
func TestManualStrengthActivityWithExerciseSets(t *testing.T) {
	writeEnabled(t)
	c := testClient(t)
	ctx := testCtx(t)

	start := time.Now().Add(-20 * time.Minute)
	act, err := c.Activities.CreateManual(ctx, garmin.ManualActivity{
		Name:     "go-garmin integration test (safe to delete)",
		TypeKey:  garmin.SportStrengthTraining.SportTypeKey,
		Start:    start,
		TimeZone: "Europe/Paris",
		Duration: 600,
	})
	if err != nil {
		t.Fatalf("CreateManual: %v", err)
	}
	t.Cleanup(func() {
		if err := c.Activities.Delete(context.Background(), act.ActivityID); err != nil {
			t.Logf("cleanup: delete activity %d: %v", act.ActivityID, err)
		}
	})

	benchPress := "BARBELL_BENCH_PRESS"
	pullUp := "WEIGHTED_PULL_UP"
	reps1, reps2 := 10, 8
	t1 := start
	t2 := start.Add(210 * time.Second)
	sets := []garmin.ExerciseSet{
		{
			SetType: garmin.ExerciseSetActive, Duration: 30 * time.Second,
			RepetitionCount: &reps1, WeightGrams: 50000,
			Exercises: []garmin.ExerciseSetExercise{{Category: "BENCH_PRESS", Name: &benchPress}},
			StartTime: &t1,
		},
		{SetType: garmin.ExerciseSetRest, Duration: 60 * time.Second, WeightGrams: -1},
		{
			SetType: garmin.ExerciseSetActive, Duration: 24 * time.Second,
			RepetitionCount: &reps2, WeightGrams: 0, // bodyweight
			Exercises: []garmin.ExerciseSetExercise{{Category: "PULL_UP", Name: &pullUp}},
			StartTime: &t2,
		},
	}
	if err := c.Activities.SetTypedExerciseSets(ctx, act.ActivityID, sets); err != nil {
		t.Fatalf("SetTypedExerciseSets: %v", err)
	}

	raw, err := c.Activities.ExerciseSets(ctx, act.ActivityID)
	if err != nil {
		t.Fatalf("ExerciseSets readback: %v", err)
	}
	var got struct {
		ExerciseSets []struct {
			SetType         string `json:"setType"`
			RepetitionCount *int   `json:"repetitionCount"`
			Weight          float64
			Exercises       []struct {
				Category string
				Name     *string
			}
		} `json:"exerciseSets"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal readback: %v", err)
	}
	if len(got.ExerciseSets) != 3 {
		t.Fatalf("got %d exercise sets, want 3: %s", len(got.ExerciseSets), raw)
	}
	first := got.ExerciseSets[0]
	if first.SetType != "ACTIVE" || first.RepetitionCount == nil || *first.RepetitionCount != 10 ||
		first.Weight != 50000 || len(first.Exercises) != 1 || first.Exercises[0].Category != "BENCH_PRESS" ||
		first.Exercises[0].Name == nil || *first.Exercises[0].Name != benchPress {
		t.Errorf("first set mismatch: %+v", first)
	}
	rest := got.ExerciseSets[1]
	if rest.SetType != "REST" || rest.Weight != -1 || len(rest.Exercises) != 0 {
		t.Errorf("rest set mismatch: %+v", rest)
	}
	third := got.ExerciseSets[2]
	if third.SetType != "ACTIVE" || third.RepetitionCount == nil || *third.RepetitionCount != 8 || third.Weight != 0 {
		t.Errorf("third set mismatch (bodyweight): %+v", third)
	}
}

func TestActivityRenameCycle(t *testing.T) {
	writeEnabled(t)
	c := testClient(t)
	ctx := testCtx(t)
	last, err := c.Activities.Last(ctx)
	if errors.Is(err, garmin.ErrNotFound) {
		t.Skip("no activities")
	}
	if err != nil {
		t.Fatalf("Last: %v", err)
	}
	orig := last.ActivityName
	if err := c.Activities.SetName(ctx, last.ActivityID, orig+" [go-garmin test]"); err != nil {
		t.Fatalf("SetName: %v", err)
	}
	// Always restore.
	if err := c.Activities.SetName(ctx, last.ActivityID, orig); err != nil {
		t.Fatalf("restoring name: %v", err)
	}
}
