package detect

import (
	"strconv"
	"time"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

// ActiveProbe detects failing or slow user-configured probes, and Service
// ports that stopped accepting connections when automatic probing is on.
type ActiveProbe struct{}

// Name implements signal.Detector.
func (ActiveProbe) Name() string { return "active-probe" }

// Kinds implements signal.Detector.
func (ActiveProbe) Kinds() []knowledge.Kind {
	return []knowledge.Kind{kube.KindEndpoint, kube.KindService}
}

// Detect implements signal.Detector.
func (ActiveProbe) Detect(
	ctx signal.Context, e knowledge.Entity,
) []signal.Signal {
	healthy, known := e.Attribute(kube.AttrHealthy)
	if !known {
		return nil
	}
	if ok, _ := healthy.Value.AsBool(); ok {
		return probeLatency(e)
	}
	seconds, _ := number(e, kube.AttrFailureDuration)
	if !sustained(ctx, healthy.Since, time.Duration(seconds)*time.Second) {
		return nil
	}
	what := "Probe " + e.ID.Name
	if e.ID.Kind == kube.KindService {
		what = "Service port"
	}
	return []signal.Signal{{
		Reason: constant.ReasonActiveProbeFailure, Severity: signal.Critical,
		Since:   healthy.Since,
		Summary: what + " is unreachable",
		Evidence: []signal.Evidence{{
			Label: "error", Value: text(e, kube.AttrProbeError),
		}},
	}}
}

func probeLatency(e knowledge.Entity) []signal.Signal {
	latency, ok := number(e, kube.AttrLatencyMS)
	if !ok {
		return nil
	}
	warn, hasWarn := number(e, kube.AttrLatencyWarnMS)
	crit, hasCrit := number(e, kube.AttrLatencyCritMS)
	severity := signal.Severity(0)
	switch {
	case hasCrit && latency >= crit:
		severity = signal.Critical
	case hasWarn && latency >= warn:
		severity = signal.Warning
	default:
		return nil
	}
	return []signal.Signal{{
		Reason: constant.ReasonActiveProbeLatency, Severity: severity,
		Since: valueSince(e, kube.AttrLatencyMS),
		Summary: "Probe " + e.ID.Name + " responds slowly (" +
			strconv.Itoa(int(latency)) + " ms)",
	}}
}
