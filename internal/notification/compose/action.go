package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/notification"
)

// fixWords introduce a rollback, by how sure the cause is. A rollback
// is only offered when the cause is at least likely, and is labelled
// as a change: every command that changes the cluster says so.
var fixWords = map[certainty]string{
	certaintyHigh:   "Rolling back fixes it (changes the cluster): ",
	certaintyLikely: "Rolling back should fix it (changes the cluster): ",
}

// actionSentences suggest the one most useful command: a labelled
// rollback when the blamed change is known, otherwise the first
// read-only command. Commands end the sentence so they copy cleanly.
func actionSentences(f caseFacts) []sentence {
	steps := nextSteps(f.p, stepMembers(f.p, f.members))
	if step, ok := rollbackStep(steps); ok {
		if words, sure := fixWords[certaintyOf(f.p.Cause)]; sure {
			return []sentence{{part: partAction,
				text: words + step.Command}}
		}
	}
	for _, step := range steps {
		if !step.Mutating && step.Command != "" {
			return []sentence{{part: partAction,
				text: "To " + lowerFirst(step.Text) + ", run " +
					step.Command}}
		}
	}
	return nil
}

func rollbackStep(steps []notification.Step) (notification.Step, bool) {
	for _, step := range steps {
		if step.Mutating && strings.Contains(step.Command, "rollout undo") {
			return step, true
		}
	}
	return notification.Step{}, false
}
