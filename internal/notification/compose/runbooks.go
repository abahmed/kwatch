package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/notification"
)

// runbookSteps links the configured runbook of each distinct member
// reason, sorted by reason for a stable message.
func (w Writer) runbookSteps(members []detection.Finding) []notification.Step {
	if len(w.Runbooks) == 0 {
		return nil
	}
	reasons := map[string]bool{}
	for _, s := range members {
		if _, ok := w.Runbooks[strings.ToLower(s.Reason)]; ok {
			reasons[s.Reason] = true
		}
	}
	var steps []notification.Step
	for _, reason := range sortedSet(reasons) {
		url := w.Runbooks[strings.ToLower(reason)]
		steps = append(steps, notification.Step{
			Text: "Runbook for " + reason + ": " + url,
		})
	}
	return steps
}
