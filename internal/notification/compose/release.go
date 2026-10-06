package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
)

// releaseRegression returns the root's own finding when it says a new
// rollout restarts more than the one it replaced.
func releaseRegression(f caseFacts) *detection.Finding {
	own := rootFinding(f.p, f.members)
	if own == nil || own.Mode != detection.ModeReleaseRegression {
		return nil
	}
	return own
}

// releaseLead leads with the detector's own sentence, which already
// compares the two rollouts: "checkout in shop: rollout 14 restarted 7
// times in its first 10 minutes, 5 times as often as rollout 13 did".
func releaseLead(f caseFacts, own detection.Finding) string {
	summary := strings.TrimSuffix(strings.TrimSpace(own.Summary), ".")
	return f.leadName(own.Entity) + ": " + lowerFirst(summary)
}
