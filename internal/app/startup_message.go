package app

import (
	"fmt"
	"strings"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/pipeline"
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
	if !s.shouldNotify || s.defersToGapReport() {
		return "", false
	}
	return s.sentence(), true
}

// defersToGapReport reports that the startup message waits for the gap
// report, which the pipeline writes once the sources synced, so the
// two go out as one message.
func (s *startupManager) defersToGapReport() bool {
	return s.gapReport && s.downtime > 0
}

// GapMessage is the startup message with the gap report; false when
// no startup message is due.
func (s *startupManager) GapMessage(report string) (string, bool) {
	if !s.shouldNotify || !s.defersToGapReport() ||
		!s.gapSent.CompareAndSwap(false, true) {
		return "", false
	}
	text := s.sentence()
	for _, marker := range []string{
		notification.MarkerLow, notification.MarkerNotify,
	} {
		text = strings.TrimPrefix(text, marker+" ")
	}
	return notification.MarkerNotify + " " + text + "\n\n" + report, true
}

func (s *startupManager) sentence() string {
	return startupSentence(startupFacts{
		version: s.currentVersion, restartReason: s.restartReason,
		now: s.now(), downtime: s.downtime, stateReset: s.stateReset,
		gapReport: s.gapReport, hideProHints: s.hideProHints,
	})
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
	// gapReport leaves the gap to the gap report sent after the sync.
	gapReport bool
	// hideProHints drops the Pro sentence after the gap.
	hideProHints bool
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
	if f.downtime > 0 && !f.gapReport {
		marker = notification.MarkerNotify
		text += gapClause(f)
		klog.InfoS("monitoring gap detected", "component", "startup",
			"operation", "announce",
			"downtime", f.downtime.Round(time.Minute))
	}
	return marker + " " + text + "."
}

// proGapHint follows the gap sentence in Community.
const proGapHint = " Pro adds a gap report: what changed and what " +
	"crashed meanwhile. Free during early access"

// gapClause says the gap went unreported and, unless hints are hidden,
// what Pro adds.
func gapClause(f startupFacts) string {
	start, end := pipeline.Gap{
		Start: f.now.Add(-f.downtime), End: f.now,
	}.Times()
	clause := fmt.Sprintf("; nothing was monitored from %s to %s UTC "+
		"(%s), so anything that broke then went unreported",
		start, end, format.Duration(f.downtime.Round(time.Minute)))
	if !f.hideProHints {
		clause += "." + proGapHint
	}
	return clause
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
