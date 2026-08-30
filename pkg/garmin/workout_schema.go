package garmin

// Typed model of the workout-service JSON schema — the payload accepted by
// WorkoutsService.Create/Update and returned by Get. The shape mirrors what
// Garmin Connect's own web app sends; every constant below was VERIFIED
// against the live API (a probe workout was created with the id/key pair and
// accepted, or the pair was observed in a server response). This is still the
// unofficial connectapi: the schema can change without notice.
//
// A workout is sportType + segments; each segment is an ordered list of steps.
// A step is either executable (type "ExecutableStepDTO": one effort, ended by
// an end condition, optionally targeted) or a repeat group (type
// "RepeatGroupDTO": numberOfIterations over child steps). Strength steps
// additionally carry an exercise (category + exerciseName, see exercises.go),
// a rep-based end condition and an optional prescribed weight.

// WorkoutSportType identifies the sport of a workout or segment.
type WorkoutSportType struct {
	SportTypeID  int    `json:"sportTypeId"`
	SportTypeKey string `json:"sportTypeKey"`
	DisplayOrder int    `json:"displayOrder,omitempty"`
}

// Workout sport types (all verified against the live API).
var (
	SportRunning          = WorkoutSportType{SportTypeID: 1, SportTypeKey: "running"}
	SportCycling          = WorkoutSportType{SportTypeID: 2, SportTypeKey: "cycling"}
	SportOther            = WorkoutSportType{SportTypeID: 3, SportTypeKey: "other"}
	SportSwimming         = WorkoutSportType{SportTypeID: 4, SportTypeKey: "swimming"}
	SportStrengthTraining = WorkoutSportType{SportTypeID: 5, SportTypeKey: "strength_training"}
	SportCardioTraining   = WorkoutSportType{SportTypeID: 6, SportTypeKey: "cardio_training"}
	SportYoga             = WorkoutSportType{SportTypeID: 7, SportTypeKey: "yoga"}
	SportPilates          = WorkoutSportType{SportTypeID: 8, SportTypeKey: "pilates"}
	SportHIIT             = WorkoutSportType{SportTypeID: 9, SportTypeKey: "hiit"}
)

// WorkoutStepType classifies a step within a workout.
type WorkoutStepType struct {
	StepTypeID   int    `json:"stepTypeId"`
	StepTypeKey  string `json:"stepTypeKey"`
	DisplayOrder int    `json:"displayOrder,omitempty"`
}

// Workout step types (verified).
var (
	StepWarmup   = WorkoutStepType{StepTypeID: 1, StepTypeKey: "warmup"}
	StepCooldown = WorkoutStepType{StepTypeID: 2, StepTypeKey: "cooldown"}
	StepInterval = WorkoutStepType{StepTypeID: 3, StepTypeKey: "interval"}
	StepRecovery = WorkoutStepType{StepTypeID: 4, StepTypeKey: "recovery"}
	StepRest     = WorkoutStepType{StepTypeID: 5, StepTypeKey: "rest"}
	StepRepeat   = WorkoutStepType{StepTypeID: 6, StepTypeKey: "repeat"}
)

// WorkoutEndCondition says what terminates a step.
type WorkoutEndCondition struct {
	ConditionTypeID  int    `json:"conditionTypeId"`
	ConditionTypeKey string `json:"conditionTypeKey"`
	DisplayOrder     int    `json:"displayOrder,omitempty"`
	Displayable      bool   `json:"displayable,omitempty"`
}

// Workout end conditions. lap.button, time, reps and iterations are verified
// against the live API; distance and calories follow the same catalogue
// (observed in Garmin Connect's own workouts).
var (
	EndLapButton  = WorkoutEndCondition{ConditionTypeID: 1, ConditionTypeKey: "lap.button"}
	EndTime       = WorkoutEndCondition{ConditionTypeID: 2, ConditionTypeKey: "time"}     // endConditionValue: seconds
	EndDistance   = WorkoutEndCondition{ConditionTypeID: 3, ConditionTypeKey: "distance"} // endConditionValue: meters
	EndCalories   = WorkoutEndCondition{ConditionTypeID: 4, ConditionTypeKey: "calories"}
	EndIterations = WorkoutEndCondition{ConditionTypeID: 7, ConditionTypeKey: "iterations"} // repeat groups
	EndReps       = WorkoutEndCondition{ConditionTypeID: 10, ConditionTypeKey: "reps"}      // strength sets
)

// WorkoutTargetType is the in-step target the device coaches against.
type WorkoutTargetType struct {
	WorkoutTargetTypeID  int    `json:"workoutTargetTypeId"`
	WorkoutTargetTypeKey string `json:"workoutTargetTypeKey"`
	DisplayOrder         int    `json:"displayOrder,omitempty"`
}

// Workout target types. no.target is verified; the zone/range targets follow
// Garmin Connect's catalogue and use targetValueOne/targetValueTwo (range) or
// zoneNumber (zone).
var (
	TargetNone      = WorkoutTargetType{WorkoutTargetTypeID: 1, WorkoutTargetTypeKey: "no.target"}
	TargetPowerZone = WorkoutTargetType{WorkoutTargetTypeID: 2, WorkoutTargetTypeKey: "power.zone"}
	TargetCadence   = WorkoutTargetType{WorkoutTargetTypeID: 3, WorkoutTargetTypeKey: "cadence.zone"}
	TargetHRZone    = WorkoutTargetType{WorkoutTargetTypeID: 4, WorkoutTargetTypeKey: "heart.rate.zone"}
	TargetSpeedZone = WorkoutTargetType{WorkoutTargetTypeID: 5, WorkoutTargetTypeKey: "speed.zone"}
	TargetPaceZone  = WorkoutTargetType{WorkoutTargetTypeID: 6, WorkoutTargetTypeKey: "pace.zone"}
)

// WorkoutWeightUnit qualifies a strength step's prescribed weight. The live
// API stores the weight only when a unit is provided (a bare weightValue is
// silently dropped).
type WorkoutWeightUnit struct {
	UnitID  int     `json:"unitId"`
	UnitKey string  `json:"unitKey"`
	Factor  float64 `json:"factor,omitempty"`
}

// Weight units (kilogram verified against the live API).
var (
	WeightKilogram = WorkoutWeightUnit{UnitID: 8, UnitKey: "kilogram", Factor: 1000}
	WeightPound    = WorkoutWeightUnit{UnitID: 9, UnitKey: "pound", Factor: 453.59237}
)

// WorkoutStep is one node of a workout's step tree: an executable step or a
// repeat group, discriminated by Type. Executable fields are pointers so an
// untouched field is omitted from the payload rather than sent as a zero the
// server might interpret.
type WorkoutStep struct {
	Type      string          `json:"type"` // StepTypeExecutable | StepTypeRepeatGroup
	StepID    int64           `json:"stepId,omitempty"`
	StepOrder int             `json:"stepOrder"`
	StepType  WorkoutStepType `json:"stepType"`
	// ChildStepID groups the steps belonging to one repeat group in server
	// responses.
	ChildStepID *int   `json:"childStepId,omitempty"`
	Description string `json:"description,omitempty"`

	// Executable-step fields.
	EndCondition      *WorkoutEndCondition `json:"endCondition,omitempty"`
	EndConditionValue *float64             `json:"endConditionValue,omitempty"`
	TargetType        *WorkoutTargetType   `json:"targetType,omitempty"`
	TargetValueOne    *float64             `json:"targetValueOne,omitempty"`
	TargetValueTwo    *float64             `json:"targetValueTwo,omitempty"`
	ZoneNumber        *int                 `json:"zoneNumber,omitempty"`

	// Strength-step fields: the exercise identity (see exercises.go for the
	// valid category/name pairs) and the prescribed weight.
	Category     string             `json:"category,omitempty"`
	ExerciseName string             `json:"exerciseName,omitempty"`
	WeightValue  *float64           `json:"weightValue,omitempty"`
	WeightUnit   *WorkoutWeightUnit `json:"weightUnit,omitempty"`

	// Repeat-group fields.
	NumberOfIterations *int          `json:"numberOfIterations,omitempty"`
	SmartRepeat        *bool         `json:"smartRepeat,omitempty"`
	SkipLastRestStep   *bool         `json:"skipLastRestStep,omitempty"`
	WorkoutSteps       []WorkoutStep `json:"workoutSteps,omitempty"`
}

// Step type discriminators.
const (
	StepTypeExecutable  = "ExecutableStepDTO"
	StepTypeRepeatGroup = "RepeatGroupDTO"
)

// WorkoutSegment is one sport segment of a workout (multisport workouts chain
// several; single-sport workouts have exactly one).
type WorkoutSegment struct {
	SegmentOrder int              `json:"segmentOrder"`
	SportType    WorkoutSportType `json:"sportType"`
	WorkoutSteps []WorkoutStep    `json:"workoutSteps"`
}

// Workout is the typed workout-service payload. Server-assigned fields
// (WorkoutID, dates) are zero on creation. Unknown response fields (author,
// estimated durations…) are deliberately not modelled and are dropped on a
// Get→Update round trip.
type Workout struct {
	WorkoutID       int64            `json:"workoutId,omitempty"`
	WorkoutName     string           `json:"workoutName"`
	Description     string           `json:"description,omitempty"`
	SportType       WorkoutSportType `json:"sportType"`
	WorkoutSegments []WorkoutSegment `json:"workoutSegments"`
}

// Float64 returns a pointer to v — a convenience for the optional numeric
// fields of WorkoutStep.
func Float64(v float64) *float64 { return &v }

// Int returns a pointer to v.
func Int(v int) *int { return &v }

// Bool returns a pointer to v.
func Bool(v bool) *bool { return &v }
