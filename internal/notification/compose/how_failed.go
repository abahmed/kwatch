package compose

import (
	"strconv"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
)

// weightHowFailed ranks how a restarting container ended. It sits below
// every other proof, which says more about why, so it fills a free
// slot of the proof and never displaces one.
const weightHowFailed = 0.05

// howFailedSentences say how long the last run lasted before it ended
// and how fast the restarts came, from numbers the detector attached.
// They describe the past only.
func howFailedSentences(f caseFacts) []sentence {
	for _, m := range f.members {
		name := ""
		if !f.aboutLead(m.Entity) {
			name = f.memberName(m.Entity)
		}
		if text := howFailedText(m, name); text != "" {
			return []sentence{{part: partProof, weight: weightHowFailed,
				text: text}}
		}
	}
	return nil
}

// howFailedText writes "Its last run lasted ten seconds; 14 restarts
// since its pod was created two hours ago." from whichever facts exist.
// A member that is not the lead's own subject is named instead of "its":
// "The last run of container app in pod api-7d9f lasted ten seconds".
func howFailedText(m detection.Finding, name string) string {
	ran := evidence(m, detection.EvidenceRanFor)
	span := evidence(m, detection.EvidenceRestartSpan)
	restarts := evidence(m, "restarts")
	var parts []string
	if ran != "" {
		parts = append(parts, lastRunText(name)+" lasted "+
			spokenSeconds(ran))
	}
	if text := restartsText(restarts, span, name != ""); text != "" {
		parts = append(parts, text)
	}
	if len(parts) == 0 {
		return ""
	}
	text := strings.Join(parts, "; ")
	return strings.ToUpper(text[:1]) + text[1:] + "."
}

// lastRunText is "its last run", or "the last run of NAME".
func lastRunText(name string) string {
	if name == "" {
		return "its last run"
	}
	return "the last run of " + name
}

// restartsText is "14 restarts since its pod was created two hours
// ago", or "" without a count and the span since the pod was created.
func restartsText(restarts, span string, named bool) string {
	count, err := strconv.Atoi(restarts)
	d, spanErr := time.ParseDuration(span)
	if err != nil || spanErr != nil {
		return ""
	}
	pod := "its pod"
	if named {
		pod = "the pod"
	}
	return plural(count, "restart") + " since " + pod + " was created " +
		humanDuration(d) + " ago"
}
