package app

import (
	"fmt"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/notification"
)

// welcomeMessage is the plain startup sentence, without a marker, for
// the log. The delivered message adds its marker and what was missed.
const welcomeMessage = "kwatch %s started"

// StartupMessage returns the one-time startup message when the application
// should notify operators. The startup session owns the state decision;
// delivery owns the transport.
//
// The message is one plain sentence with one status marker and no links:
// 🟡 for a normal start, 🟠 when monitoring stopped unexpectedly or a gap
// went unwatched, because something may have broken unreported.
func (s *startupManager) StartupMessage() (string, bool) {
	if !s.shouldNotify {
		return "", false
	}
	return startupSentence(startupFacts{
		version: s.currentVersion, restartReason: s.restartReason,
		now: s.now(), downtime: s.downtime, stateReset: s.stateReset,
	}), true
}

// startupFacts are what the startup sentence reports.
type startupFacts struct {
	version string
	// restartReason is empty after a clean stop.
	restartReason string
	now           time.Time
	// downtime is zero when no gap was worth reporting.
	downtime time.Duration
	// stateReset is true when the state file was unusable and was
	// replaced: earlier incidents are forgotten and will be announced
	// again.
	stateReset bool
}

// startupSentence writes the startup message.
func startupSentence(f startupFacts) string {
	marker := notification.MarkerLow
	text := fmt.Sprintf(welcomeMessage, f.version)
	reason := f.restartReason
	if reason == "graceful_shutdown" {
		// A clean stop is not worth a reason: it is a plain start.
		reason = ""
	}
	if reason != "" {
		text += " again after " + humanRestartReason(reason)
		if reason == "internal_failure" {
			marker = notification.MarkerNotify
		}
	}
	if f.stateReset {
		marker = notification.MarkerNotify
		text += " with fresh state: earlier incident history was reset, " +
			"so open problems will be announced again"
	}
	now, downtime := f.now, f.downtime
	if downtime > 0 {
		marker = notification.MarkerNotify
		gapStart := now.Add(-downtime)
		text += fmt.Sprintf("; nothing was monitored from %s to %s UTC "+
			"(%s), so anything that broke then went unreported",
			gapStart.UTC().Format("15:04"), now.UTC().Format("15:04"),
			format.Duration(downtime.Round(time.Minute)))
		klog.InfoS("monitoring gap detected", "component", "startup",
			"operation", "announce",
			"downtime", downtime.Round(time.Minute))
	}
	return marker + " " + text + "."
}

func humanRestartReason(reason string) string {
	switch reason {
	case "node_disruption":
		return "a node disruption"
	case "deployment_rollout":
		return "a kwatch rollout"
	case "leader_handoff":
		return "a leader handoff"
	case "eviction":
		return "an eviction"
	case "oom_killed":
		return "an out-of-memory termination"
	case "api_unavailable":
		return "a Kubernetes API outage"
	case "internal_failure":
		return "an unexpected kwatch stop"
	default:
		return "an interrupted monitoring session"
	}
}
