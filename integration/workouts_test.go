//go:build integration

package integration

import (
	"testing"

	"github.com/ndeloof/go-garmin/pkg/garmin"
)

// The typed workout schema must round-trip through the live API: a strength
// workout built from the typed model (repeat group, rep-based sets, exercise
// taxonomy, prescribed weight) is created, read back and compared — then
// deleted so the account stays clean.
func TestWorkoutStrengthTypedRoundTrip(t *testing.T) {
	writeEnabled(t)
	c := testClient(t)
	ctx := testCtx(t)

	w := &garmin.Workout{
		WorkoutName: "go-garmin integration probe",
		SportType:   garmin.SportStrengthTraining,
		WorkoutSegments: []garmin.WorkoutSegment{{
			SegmentOrder: 1,
			SportType:    garmin.SportStrengthTraining,
			WorkoutSteps: []garmin.WorkoutStep{{
				Type:               garmin.StepTypeRepeatGroup,
				StepOrder:          1,
				StepType:           garmin.StepRepeat,
				NumberOfIterations: garmin.Int(4),
				WorkoutSteps: []garmin.WorkoutStep{
					{
						Type:              garmin.StepTypeExecutable,
						StepOrder:         2,
						StepType:          garmin.StepInterval,
						EndCondition:      &garmin.EndReps,
						EndConditionValue: garmin.Float64(6),
						Category:          garmin.ExerciseCategorySquat,
						ExerciseName:      "BARBELL_BACK_SQUAT",
						WeightValue:       garmin.Float64(80),
						WeightUnit:        &garmin.WeightKilogram,
					},
					{
						Type:              garmin.StepTypeExecutable,
						StepOrder:         3,
						StepType:          garmin.StepRest,
						EndCondition:      &garmin.EndTime,
						EndConditionValue: garmin.Float64(150),
					},
				},
			}},
		}},
	}
	if !garmin.ValidExercise(w.WorkoutSegments[0].WorkoutSteps[0].WorkoutSteps[0].Category,
		w.WorkoutSegments[0].WorkoutSteps[0].WorkoutSteps[0].ExerciseName) {
		t.Fatal("probe exercise missing from the generated taxonomy")
	}

	created, err := c.Workouts.CreateTyped(ctx, w)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() {
		if err := c.Workouts.Delete(ctx, created.WorkoutID); err != nil {
			t.Errorf("cleanup delete workout %d: %v", created.WorkoutID, err)
		}
	})
	if created.WorkoutID == 0 {
		t.Fatal("created workout has no id")
	}

	got, err := c.Workouts.GetTyped(ctx, created.WorkoutID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.SportType.SportTypeID != garmin.SportStrengthTraining.SportTypeID {
		t.Errorf("sportType = %+v", got.SportType)
	}
	repeat := got.WorkoutSegments[0].WorkoutSteps[0]
	if repeat.Type != garmin.StepTypeRepeatGroup || *repeat.NumberOfIterations != 4 {
		t.Fatalf("repeat = %+v", repeat)
	}
	set := repeat.WorkoutSteps[0]
	if set.Category != garmin.ExerciseCategorySquat || set.ExerciseName != "BARBELL_BACK_SQUAT" {
		t.Errorf("exercise = %s/%s", set.Category, set.ExerciseName)
	}
	if set.EndConditionValue == nil || *set.EndConditionValue != 6 {
		t.Errorf("reps = %v", set.EndConditionValue)
	}
	// The prescribed weight only survives when a unit is sent — the exact
	// regression this test guards.
	if set.WeightValue == nil || *set.WeightValue != 80 ||
		set.WeightUnit == nil || set.WeightUnit.UnitKey != garmin.WeightKilogram.UnitKey {
		t.Errorf("weight = %v %v", set.WeightValue, set.WeightUnit)
	}
	rest := repeat.WorkoutSteps[1]
	if rest.StepType.StepTypeKey != garmin.StepRest.StepTypeKey ||
		rest.EndConditionValue == nil || *rest.EndConditionValue != 150 {
		t.Errorf("rest = %+v", rest)
	}
}
