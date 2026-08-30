package garmin

//go:generate go run ../../internal/genexercises

// The Garmin Connect strength-exercise taxonomy: 47 categories, ~1500
// exercises (exercises_gen.go, generated from the public taxonomy file the
// Connect web app loads). A strength WorkoutStep carries one (Category,
// ExerciseName) pair from this taxonomy — that is what makes the watch show
// the right exercise, animation and muscle map during a guided session. An
// unknown pair is accepted by the API but renders as a generic exercise, so
// callers should validate with ValidExercise at build time.

// ExerciseCategories returns the taxonomy's category keys, sorted.
func ExerciseCategories() []string {
	out := make([]string, 0, len(exercisesByCategory))
	for _, c := range exerciseCategoryOrder {
		out = append(out, c)
	}
	return out
}

// ExercisesIn returns the exercise names of one category, sorted. Nil for an
// unknown category.
func ExercisesIn(category string) []string {
	src := exercisesByCategory[category]
	if src == nil {
		return nil
	}
	out := make([]string, len(src))
	copy(out, src)
	return out
}

// ValidExercise reports whether (category, exerciseName) is a known pair of
// the taxonomy.
func ValidExercise(category, exerciseName string) bool {
	for _, n := range exercisesByCategory[category] {
		if n == exerciseName {
			return true
		}
	}
	return false
}
