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
	return []inventory.Kind{kube.KindEndpoint, kube.KindService,
		kube.KindExternalEndpoint, kube.KindKwatch}
}

// Detect implements detection.Detector.
func (ActiveProbe) Detect(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	if e.ID.Kind == kube.KindKwatch {
		return networkRestricted(ctx, e)
	}
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
	what, state := "Probe "+e.ID.Name, "is unreachable"
	switch e.ID.Kind {
	case kube.KindService:
		what = "Service port"
	case kube.KindExternalEndpoint:
		// The entity's name is the endpoint; the lead names it.
		what, state = "Endpoint", "does not accept connections"
	}
	if words, known := failureWords(e); known {
		state = words
	}
	return []detection.Finding{{
		Reason: reasons.ActiveProbeFailure, Severity: detection.Critical,
		Since:    healthy.Since,
		Summary:  what + " " + state,
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

// failureWords say how the probe failed, when it recorded how: a
// silent target, a refusal and a missing name call for different fixes.
func failureWords(e inventory.Entity) (string, bool) {
	switch text(e, kube.AttrProbeFailureKind) {
	case kube.FailureTimeout:
		wait, ok := number(e, kube.AttrProbeTimeoutSeconds)
		if !ok {
			return "did not answer in time", true
		}
		// Whole seconds, spelled out: the message writer would turn
		// a bare "3s" into "less than a minute".
		return "did not answer within " + seconds(
			time.Duration(wait*float64(time.Second))), true
	case kube.FailureRefused:
		return "refused the connection", true
	case kube.FailureDNS:
		return "name does not resolve", true
	case kube.FailureDNSLookup:
		return "DNS lookup failed", true
	}
	return "", false
}

// networkRestricted reports that every dependency kwatch probed failed
// in the same round. The cause is more likely kwatch's own network
// (an egress policy, a missing route) than that many dependencies
// being down at once, so no dependency is blamed. It is informational.
func networkRestricted(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	failed, _ := number(e, kube.AttrDependenciesUnreachable)
	if failed < 1 {
		return nil
	}
	seconds, _ := number(e, kube.AttrFailureDuration)
	since := valueSince(e, kube.AttrDependenciesUnreachable)
	if !sustained(ctx, "kwatch-network", since, boundedSeconds(seconds)) {
		return nil
	}
	count := strconv.Itoa(int(failed))
	return []detection.Finding{{
		Reason: reasons.KwatchNetworkRestricted, Severity: detection.Info,
		Since: since,
		Summary: "kwatch could not reach any of its " + count +
			" probed dependencies; its own network may be restricted",
		Evidence: []detection.Evidence{{
			Label: "dependencies probed, all failing", Value: count}},
	}}
}
