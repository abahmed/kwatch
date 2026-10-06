package compose

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
)

// weightJobRun ranks the run times of a Job that runs long: they are
// the whole proof.
const weightJobRun = 0.7

// jobRunSentences compare a Job's run time with the recent runs of its
// CronJob: "It has run 2h10m; recent runs took 8–12m."
func jobRunSentences(f caseFacts) []sentence {
	for _, m := range f.members {
		if m.Reason != reasons.JobRunningLong {
			continue
		}
		running := evidence(m, detection.EvidenceRunningFor)
		recent := evidence(m, detection.EvidenceRecentRuns)
		if running == "" || recent == "" {
			continue
		}
		return []sentence{{part: partProof, weight: weightJobRun,
			text: "It has run " + running + "; recent runs took " +
				recent + "."}}
	}
	return nil
}
