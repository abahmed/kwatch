package detectors

import (
	"math"
	"strconv"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// ActiveProbe detects failing or slow user-configured probes, and Service
// ports that stopped accepting connections when automatic probing is on.
type ActiveProbe struct{}

// Name implements detection.Detector.
func (ActiveProbe) Name() string { return "active-probe" }

// Kinds implements detection.Detector.
func (ActiveProbe) Kinds() []inventory.Kind {
	return []inventory.Kind{kube.KindEndpoint, kube.KindService}
}

// Detect implements detection.Detector.
func (ActiveProbe) Detect(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	healthy, known := e.Attribute(kube.AttrHealthy)
	if !known {
		return nil
	}
	if ok, _ := healthy.Value.AsBool(); ok {
		return probeLatency(e)
	}
	seconds, _ := number(e, kube.AttrFailureDuration)
	failFor := boundedSeconds(seconds)
	if !sustained(ctx, "probe-failing", healthy.Since, failFor) {
		return nil
	}
	what := "Probe " + e.ID.Name
	if e.ID.Kind == kube.KindService {
		what = "Service port"
	}
	return []detection.Finding{{
		Reason: reasons.ActiveProbeFailure, Severity: detection.Critical,
		Since:    healthy.Since,
		Summary:  what + " is unreachable",
		Evidence: errorEvidence(e),
	}}
}

// maxProbeWait caps a configured failure duration so the conversion to
// time.Duration cannot overflow.
const maxProbeWait = 24 * time.Hour

// boundedSeconds converts seconds to a duration between zero and
// maxProbeWait.
func boundedSeconds(seconds float64) time.Duration {
	switch {
	case math.IsNaN(seconds) || seconds <= 0:
		return 0
	case seconds >= maxProbeWait.Seconds():
		return maxProbeWait
	}
	return time.Duration(seconds * float64(time.Second))
}

// errorEvidence returns the probe error as evidence, or none when the
// probe reported no error text.
func errorEvidence(e inventory.Entity) []detection.Evidence {
	message := text(e, kube.AttrProbeError)
	if message == "" {
		return nil
	}
	return []detection.Evidence{{Label: "error", Value: message}}
}

func probeLatency(e inventory.Entity) []detection.Finding {
	latency, ok := number(e, kube.AttrLatencyMS)
	if !ok {
		return nil
	}
	warn, hasWarn := number(e, kube.AttrLatencyWarnMS)
	crit, hasCrit := number(e, kube.AttrLatencyCritMS)
	severity := detection.Severity(0)
	switch {
	case hasCrit && latency >= crit:
		severity = detection.Critical
	case hasWarn && latency >= warn:
		severity = detection.Warning
	default:
		return nil
	}
	return []detection.Finding{{
		Reason: reasons.ActiveProbeLatency, Severity: severity,
		Since: valueSince(e, kube.AttrLatencyMS),
		Summary: "Probe " + e.ID.Name + " responds slowly (" +
			strconv.Itoa(int(latency)) + " ms)",
	}}
}
