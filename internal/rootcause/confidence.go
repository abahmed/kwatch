package rootcause

// This file is the one place a cause's score becomes a word. Chat
// messages, the dashboard, explain's traces and the scorecard all ask
// it, so the same incident is never "likely" in one and "possible" in
// another.

// Confidence thresholds shown to readers. High was calibrated on the
// labelled scenarios (scorecard.CalibratedHigh): causes scored 0.70
// were right 92% of the time, so a cause scored that high is stated
// plainly. The scorecard gates hold it (calibration_boundary_test.go in
// internal/scenarios): the build fails when the boundary the labelled
// cases give moves more than 0.05 above High or 0.15 below it, or when
// the labelled causes scored at or above High are right less than 92%
// of the time. At Likely a cause is worded as likely; below it kwatch
// only suggests.
const (
	High   = 0.70
	Likely = 0.5
	// RivalMargin is how close two causes of the same failures must
	// score to be told as two possible causes instead of one answer.
	RivalMargin = 0.05
	// RivalCap is the highest score of a cause that has a rival: just
	// under High, since a message names both causes.
	RivalCap = High - 0.01
)

// Level words how sure a cause is.
type Level string

// Confidence levels, loudest first. LevelNone is no cause at all.
const (
	LevelHigh     Level = "high"
	LevelLikely   Level = "likely"
	LevelPossible Level = "possible"
	LevelNone     Level = ""
)

// LevelOf words a bare score, with no rival to consider.
func LevelOf(score float64) Level {
	switch {
	case score >= High:
		return LevelHigh
	case score >= Likely:
		return LevelLikely
	}
	return LevelPossible
}

// CertaintyOf is the level a stated cause is worded at. A cause with a
// rival of nearly the same score is only possible, whatever its score:
// kwatch cannot tell the two apart, so it must not pick one.
func CertaintyOf(cause *CauseRecord) Level {
	switch {
	case cause == nil:
		return LevelNone
	case cause.Rival != nil:
		return LevelPossible
	}
	return LevelOf(cause.Score)
}

// MinCorroboration is how many pieces of supporting evidence a cause
// below Likely needs, besides the rule that blamed it, before a
// message may name it: a counterfactual check (Checked) or a proof
// for it (Proof with Supports) each count once.
const MinCorroboration = 1

// Confirmed reports whether a message may name the cause as the cause.
// A cause at LevelPossible is not confirmed when a rival scored almost
// alike, or when nothing corroborates it (MinCorroboration): kwatch
// then says the symptom and that it could not confirm a cause, and the
// candidate is shown only as a possibility. A cause at Likely or High
// is always confirmed.
func Confirmed(cause *CauseRecord) bool {
	if cause == nil {
		return false
	}
	if CertaintyOf(cause) != LevelPossible {
		return true
	}
	return cause.Rival == nil && corroboration(cause) >= MinCorroboration
}

// corroboration counts the evidence for a cause beyond its rule.
func corroboration(cause *CauseRecord) int {
	count := len(cause.Checked)
	for _, p := range cause.Proof {
		if p.Supports {
			count++
		}
	}
	return count
}
