package compose

import (
	"strconv"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// changedShortly is the table wording of a change whose gap to the
// first failure is not known.
const changedShortly = "changed shortly before"

// measuredGap says how long before the failures began the cause
// changed: "changed 3 min before the pods started failing". Words that
// are not the change wording, or a cause with no measured gap, are
// returned as they are.
func measuredGap(cause *rootcause.CauseRecord, words string) string {
	if words != changedShortly || cause.ChangeGap == nil {
		return words
	}
	return "changed " + gapText(*cause.ChangeGap) + " before " +
		failingThing(cause) + " started failing"
}

// gapText writes a gap as "seconds" or "N min".
func gapText(d time.Duration) string {
	if d < time.Minute {
		return "seconds"
	}
	return strconv.Itoa(int(d.Round(time.Minute)/time.Minute)) + " min"
}

// failingThing names what started failing, from the symptom the
// cause's chain ends at.
func failingThing(cause *rootcause.CauseRecord) string {
	if n := len(cause.Chain); n > 0 {
		switch cause.Chain[n-1].Kind {
		case kube.KindJob, kube.KindCronJob:
			return "the job"
		}
	}
	if strings.Contains(cause.Rule, "dns") {
		return "lookups"
	}
	return "the pods"
}
