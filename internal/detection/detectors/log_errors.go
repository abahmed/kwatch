package detectors

import (
	"fmt"
	"strconv"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Error burst thresholds. They are constants, not settings: a burst is
// judged against the workload's own earlier windows, so one floor and
// one factor fit any service.
const (
	// ErrorBurstFloor is the fewest error lines in one sample window
	// that can be a burst. Fewer are noise on any service.
	ErrorBurstFloor = 20
	// ErrorBurstFactor is how many times the workload's earlier error
	// rate per window the last window must reach.
	ErrorBurstFactor = 5.0
	// ErrorBaselineMin is the least history a workload needs before a
	// burst is judged: without it, a service that always logs this many
	// errors would be flagged right after kwatch started.
	ErrorBaselineMin = 10 * time.Minute
)

// LogErrors detects a workload that Kubernetes sees as healthy while its
// pods' own log reports a burst of errors new against its earlier
// windows. It reads the attributes kube.LogSampleRound writes and
// states only what the last window held.
type LogErrors struct{}

// Name implements detection.Detector.
func (LogErrors) Name() string { return "log-errors" }

// Kinds implements detection.Detector.
func (LogErrors) Kinds() []inventory.Kind {
	return []inventory.Kind{kube.KindDeployment, kube.KindStatefulSet,
		kube.KindDaemonSet}
}

// Detect implements detection.Detector.
func (LogErrors) Detect(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	errors, ok := number(e, kube.AttrLogErrors)
	if !ok || errors < ErrorBurstFloor || !servingFully(e) {
		return nil
	}
	window, _ := number(e, kube.AttrLogWindow)
	baseline, _ := number(e, kube.AttrLogBaseline)
	span, _ := number(e, kube.AttrLogBaselineSpan)
	if window <= 0 || span < ErrorBaselineMin.Seconds() {
		return nil
	}
	rate := baseline / (span / window)
	if errors < ErrorBurstFactor*max(rate, 1) {
		return nil
	}
	since := ctx.Onset("log-error-burst", valueSince(e, kube.AttrLogErrors))
	return []detection.Finding{{
		Reason: reasons.LogErrorBurst, Severity: detection.Warning,
		Since:    since,
		Summary:  burstSummary(errors, window),
		Evidence: burstEvidence(e, errors, window, span),
	}}
}

// servingFully reports a workload with every replica ready: the case
// where Kubernetes has nothing to say.
func servingFully(e inventory.Entity) bool {
	desired, ok := desiredReplicas(e)
	ready, _ := number(e, kube.AttrReadyReplicas)
	return ok && desired > 0 && ready >= desired
}

// burstSummary is "340 errors in 5 min".
func burstSummary(errors, window float64) string {
	return fmt.Sprintf("%d errors in %d min", int(errors),
		max(int(window/60), 1))
}

func burstEvidence(
	e inventory.Entity, errors, window, span float64,
) []detection.Evidence {
	out := []detection.Evidence{
		{Label: detection.EvidenceLogErrors, Value: itoa(errors)},
		{Label: detection.EvidenceLogWindow, Value: itoa(window)},
		{Label: detection.EvidenceLogBaselineSpan, Value: itoa(span)},
	}
	if line := text(e, kube.AttrLogTopLine); line != "" {
		count, _ := number(e, kube.AttrLogTopCount)
		out = append(out,
			detection.Evidence{Label: detection.EvidenceLogLine, Value: line},
			detection.Evidence{Label: detection.EvidenceLogLineCount,
				Value: itoa(count)})
		// Absent when an earlier window met more distinct lines than it
		// tracks: how often the line came up before is then unknown.
		if before, known := number(e, kube.AttrLogTopBaseline); known {
			out = append(out, detection.Evidence{
				Label: detection.EvidenceLogLineBefore, Value: itoa(before)})
		}
	}
	return out
}

func itoa(n float64) string { return strconv.Itoa(int(n)) }
