package garmin

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// The typed schema must decode real server responses (captured in testdata
// from the live API) and survive a marshal→unmarshal round trip unchanged —
// that is what lets a client Get, tweak and re-Create a workout.
func TestWorkoutSchemaDecodesStrengthFixture(t *testing.T) {
	data, err := os.ReadFile("testdata/workout_strength.json")
	if err != nil {
		t.Fatal(err)
	}
	var w Workout
	if err := json.Unmarshal(data, &w); err != nil {
		t.Fatal(err)
	}
	if w.SportType.SportTypeKey != SportStrengthTraining.SportTypeKey ||
		w.SportType.SportTypeID != SportStrengthTraining.SportTypeID {
		t.Fatalf("sportType = %+v", w.SportType)
	}
	if len(w.WorkoutSegments) != 1 {
		t.Fatalf("segments = %d", len(w.WorkoutSegments))
	}
	steps := w.WorkoutSegments[0].WorkoutSteps
	if len(steps) != 1 || steps[0].Type != StepTypeRepeatGroup {
		t.Fatalf("top steps = %+v", steps)
	}
	repeat := steps[0]
	if repeat.NumberOfIterations == nil || *repeat.NumberOfIterations != 3 {
		t.Errorf("iterations = %v", repeat.NumberOfIterations)
	}
	set := repeat.WorkoutSteps[0]
	if set.Category != ExerciseCategorySquat || set.ExerciseName != "BARBELL_BACK_SQUAT" {
		t.Errorf("exercise = %s/%s", set.Category, set.ExerciseName)
	}
	if set.EndCondition == nil || set.EndCondition.ConditionTypeKey != EndReps.ConditionTypeKey ||
		set.EndConditionValue == nil || *set.EndConditionValue != 10 {
		t.Errorf("end condition = %+v (%v)", set.EndCondition, set.EndConditionValue)
	}
	rest := repeat.WorkoutSteps[1]
	if rest.StepType.StepTypeKey != StepRest.StepTypeKey ||
		rest.EndCondition.ConditionTypeKey != EndTime.ConditionTypeKey {
		t.Errorf("rest step = %+v", rest)
	}

	// Round trip: marshal the decoded struct and decode it again — the two
	// structs must be identical (unknown server fields are dropped once, on
	// the first decode, by design).
	out, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	var w2 Workout
	if err := json.Unmarshal(out, &w2); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(w, w2) {
		t.Error("workout changed across a marshal/unmarshal round trip")
	}
}

func TestWorkoutSchemaDecodesRunningFixture(t *testing.T) {
	data, err := os.ReadFile("testdata/workout_running.json")
	if err != nil {
		t.Fatal(err)
	}
	var w Workout
	if err := json.Unmarshal(data, &w); err != nil {
		t.Fatal(err)
	}
	if w.SportType.SportTypeID != SportRunning.SportTypeID {
		t.Fatalf("sportType = %+v", w.SportType)
	}
	var sawWarmup, sawRepeat, sawTarget bool
	for _, s := range w.WorkoutSegments[0].WorkoutSteps {
		switch {
		case s.StepType.StepTypeKey == StepWarmup.StepTypeKey:
			sawWarmup = true
			if s.EndCondition.ConditionTypeKey != EndLapButton.ConditionTypeKey {
				t.Errorf("warmup end condition = %+v", s.EndCondition)
			}
		case s.Type == StepTypeRepeatGroup:
			sawRepeat = true
		}
		if s.TargetType != nil && s.TargetType.WorkoutTargetTypeKey == TargetNone.WorkoutTargetTypeKey {
			sawTarget = true
		}
	}
	if !sawWarmup || !sawRepeat || !sawTarget {
		t.Errorf("fixture coverage: warmup=%v repeat=%v target=%v", sawWarmup, sawRepeat, sawTarget)
	}
}

// The generated taxonomy answers the questions a workout builder asks.
func TestExerciseTaxonomy(t *testing.T) {
	cats := ExerciseCategories()
	if len(cats) != 47 {
		t.Fatalf("categories = %d, want 47", len(cats))
	}
	if !ValidExercise(ExerciseCategorySquat, "BARBELL_BACK_SQUAT") {
		t.Error("BARBELL_BACK_SQUAT missing from SQUAT")
	}
	if ValidExercise(ExerciseCategorySquat, "BARBELL_BACK_SQUATT") {
		t.Error("typo accepted")
	}
	if ValidExercise("NOT_A_CATEGORY", "BARBELL_BACK_SQUAT") {
		t.Error("unknown category accepted")
	}
	if n := len(ExercisesIn(ExerciseCategorySquat)); n < 50 {
		t.Errorf("SQUAT has %d exercises, expected dozens", n)
	}
	if ExercisesIn("NOT_A_CATEGORY") != nil {
		t.Error("unknown category should list nil")
	}
}
