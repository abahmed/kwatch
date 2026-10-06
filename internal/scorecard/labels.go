package scorecard

// Confidence floors of the levels a cause is stated with. They match the
// wording thresholds of the root-cause engine: at or above HighConfidence
// a message states the cause plainly, at or above LikelyConfidence it
// says "likely", and below that it hedges.
//
// HighConfidence was calibrated on 2026-10-01 with CalibratedHigh over
// the labelled scenarios only (never the held-out ones): the 13 cases
// scored in [0.70, 0.75) were right 12 times (92%), inside the high
// band, so a cause scored 0.70 is stated plainly. The 4 cases between
// 0.50 and 0.70 were all right, but too few to judge, so that range
// stays "likely". The report prints what the method gives today.
const (
	HighConfidence   = 0.70
	LikelyConfidence = 0.5
)

// Confidence level names, loudest first.
const (
	LevelHigh     = "high"
	LevelLikely   = "likely"
	LevelPossible = "possible"
	LevelNone     = "none"
)

// Case is the outcome of one labelled scenario: the root it expected, the
// root the engine blamed and how sure the engine said it was.
type Case struct {
	Name     string `json:"name"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
	Correct  bool   `json:"correct"`
	// Confidence is the stated confidence of the blamed cause in [0, 1].
	// Zero means no cause was stated.
	Confidence float64 `json:"confidence"`
}

// Level is the observed accuracy of every case stated at one confidence
// level.
type Level struct {
	Name string `json:"name"`
	// Floor is the lowest confidence of the level. A calibrated level is
	// right at least this often.
	Floor   float64 `json:"floor"`
	Cases   int     `json:"cases"`
	Correct int     `json:"correct"`
}

// AccuracyPercent is the share of the level's cases that were right.
func (l Level) AccuracyPercent() float64 { return percent(l.Correct, l.Cases) }

// Calibrated reports whether the level is right at least as often as it
// promises. A level without cases makes no promise. The high level
// promises CalibrationHighAccuracy, the bar CalibratedHigh holds the
// boundary to, not its own floor: a cause worded plainly at 70% would
// otherwise read as calibrated while the method itself demands 80%.
func (l Level) Calibrated() bool {
	return l.Cases == 0 || l.AccuracyPercent() >= l.promise()
}

// promise is the accuracy, in percent, the level must show.
func (l Level) promise() float64 {
	if l.Name == LevelHigh {
		return CalibrationHighAccuracy
	}
	return l.Floor * 100
}

// Accuracy is the root-cause quality of a set of labelled cases.
type Accuracy struct {
	Cases   int `json:"cases"`
	Correct int `json:"correct"`
	// WrongHigh counts wrong roots stated with high confidence.
	WrongHigh int     `json:"wrongHigh"`
	Levels    []Level `json:"levels"`
}

// CorrectPercent is the share of cases with the expected root.
func (a Accuracy) CorrectPercent() float64 {
	return percent(a.Correct, a.Cases)
}

// HighCases counts the cases stated with high confidence.
func (a Accuracy) HighCases() int {
	for _, level := range a.Levels {
		if level.Name == LevelHigh {
			return level.Cases
		}
	}
	return 0
}

// WrongHighPercent is the share of high-confidence cases that named the
// wrong root: how often a plainly stated cause is wrong.
func (a Accuracy) WrongHighPercent() float64 {
	return percent(a.WrongHigh, a.HighCases())
}

// Calibrated reports whether every confidence level is calibrated.
func (a Accuracy) Calibrated() bool {
	for _, level := range a.Levels {
		if !level.Calibrated() {
			return false
		}
	}
	return true
}

// confidenceEpsilon keeps a score that is 0.7 up to rounding (0.6999999)
// in the 0.70 level, as the calibration buckets do.
const confidenceEpsilon = 1e-9

// ConfidenceLevel names the level a confidence is stated at.
func ConfidenceLevel(confidence float64) string {
	if confidence <= 0 {
		return LevelNone
	}
	confidence += confidenceEpsilon
	switch {
	case confidence >= HighConfidence:
		return LevelHigh
	case confidence >= LikelyConfidence:
		return LevelLikely
	}
	return LevelPossible
}

// ScoreCases measures root-cause accuracy and calibration.
func ScoreCases(cases []Case) Accuracy {
	levels := []Level{
		{Name: LevelHigh, Floor: HighConfidence},
		{Name: LevelLikely, Floor: LikelyConfidence},
		{Name: LevelPossible},
		{Name: LevelNone},
	}
	out := Accuracy{Cases: len(cases)}
	for _, c := range cases {
		name := ConfidenceLevel(c.Confidence)
		if c.Correct {
			out.Correct++
		} else if name == LevelHigh {
			out.WrongHigh++
		}
		for i := range levels {
			if levels[i].Name != name {
				continue
			}
			levels[i].Cases++
			if c.Correct {
				levels[i].Correct++
			}
		}
	}
	out.Levels = levels
	return out
}
