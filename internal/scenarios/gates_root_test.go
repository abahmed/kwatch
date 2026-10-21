package scenarios

import (
	"fmt"

	"github.com/abahmed/kwatch/internal/scorecard"
)

// rootGates are the root-cause gates that count instead of averaging. The
// percentage gates let a confident wrong answer hide among many right
// ones, and judge only the final state; these do not. A scenario whose
// story is a legitimate revision opts out of the first-message gate with
// RevisesRoot; a hedge on an ambiguous label is not a wrong root.
func rootGates(
	verdicts []verdict, accuracy scorecard.Accuracy,
) []scorecard.Gate {
	first := 0
	for _, v := range verdicts {
		if v.wrongFirstRoot() {
			first++
		}
	}
	return []scorecard.Gate{
		scorecard.AtMost(
			"Wrong root stated at likely confidence or higher (labelled)",
			float64(accuracy.WrongStated),
			scorecard.GoalWrongStatedRoots, ""),
		scorecard.AtMost("Scenarios whose first message stated a wrong root",
			float64(first), scorecard.GoalWrongFirstRoots, ""),
	}
}

// foundLine says how the right roots were found, so a fallback to the
// failing entity is not mistaken for a found cause, how many wrong roots
// were stated, and how many scenarios stated one in some message (the
// gate reads the first message only).
func foundLine(a scorecard.Accuracy, verdicts []verdict) string {
	opted, later := 0, 0
	for _, v := range verdicts {
		if v.expect.RevisesRoot {
			opted++
		}
		if len(v.announcedWrong) > 0 {
			later++
		}
	}
	return fmt.Sprintf("Right roots: %d with a stated cause, %d by falling "+
		"back to the failing entity with no stated cause, %d by stating "+
		"no cause as labelled. Wrong roots stated at likely confidence or "+
		"higher: %d (%d hedges on ambiguous labels tolerated). Wrong root "+
		"stated in some message: %d scenarios, %d of them labelled as "+
		"legitimate revisions.\n", a.CorrectStated, a.CorrectFallback,
		a.CorrectUnknown, a.WrongStated, a.ToleratedHedges, later, opted)
}
