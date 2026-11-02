package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// runOf reports a Job that a CronJob started: the CronJob controller names
// its Jobs after it.
func runOf(cron, job inventory.EntityID) bool {
	return cron.Kind == kube.KindCronJob && job.Kind == kube.KindJob &&
		job.Namespace == cron.Namespace &&
		strings.HasPrefix(job.Name, cron.Name+"-")
}

// lastRunSentence says that the run of a CronJob failed, and why in the
// words of the run's own finding. A run is the CronJob doing its work, so
// the failure is not "spreading" to it. named says an earlier sentence
// already led with the CronJob.
func lastRunSentence(
	home inventory.EntityID, homeName string, named bool, g spreadGroup,
) (string, bool) {
	if len(g.ids) != 1 || len(g.states) != 1 || !runOf(home, g.ids[0]) {
		return "", false
	}
	why, failing := strings.CutPrefix(g.states[0], "is failing: ")
	if !failing {
		return "", false
	}
	text := "Its last run, "
	if !named {
		text = "The last run of " + homeName + ", "
	}
	text += nameList(home, g.ids) + ", failed: " + why
	return endSentence(text), true
}
