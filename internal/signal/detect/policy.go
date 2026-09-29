package detect

import (
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

// DefaultBudgetBlocked is how long a PDB may block every disruption before
// it matters: drains and upgrades wait on it.
const DefaultBudgetBlocked = 10 * time.Minute

// Budget detects PodDisruptionBudgets that block all disruptions because
// the pods they protect are unhealthy.
type Budget struct{}

// Name implements signal.Detector.
func (Budget) Name() string { return "disruption-budget" }

// Kinds implements signal.Detector.
func (Budget) Kinds() []knowledge.Kind { return []knowledge.Kind{kube.KindPDB} }

// Detect implements signal.Detector.
func (Budget) Detect(ctx signal.Context, e knowledge.Entity) []signal.Signal {
	allowed, _ := number(e, kube.AttrDisruptionsAllowed)
	healthy, _ := number(e, kube.AttrCurrentHealthy)
	desired, _ := number(e, kube.AttrDesiredHealthy)
	expected, _ := number(e, kube.AttrExpectedPods)
	if allowed > 0 || expected == 0 || healthy >= desired {
		return nil
	}
	since := valueSince(e, kube.AttrDisruptionsAllowed)
	if !sustained(ctx, since, DefaultBudgetBlocked) {
		return nil
	}
	return []signal.Signal{{
		Reason: constant.ReasonPdbViolation, Severity: signal.Warning,
		Since: since,
		Summary: "Disruption budget blocks node drains: too few healthy " +
			"pods",
	}}
}

// Quota detects ResourceQuotas with an exhausted resource.
type Quota struct{}

// Name implements signal.Detector.
func (Quota) Name() string { return "quota" }

// Kinds implements signal.Detector.
func (Quota) Kinds() []knowledge.Kind {
	return []knowledge.Kind{kube.KindQuota}
}

// Detect implements signal.Detector.
func (Quota) Detect(_ signal.Context, e knowledge.Entity) []signal.Signal {
	exhausted := text(e, kube.AttrExhausted)
	if exhausted == "" {
		return nil
	}
	return []signal.Signal{{
		Reason:   constant.ReasonResourceQuotaExhausted,
		Severity: signal.Warning,
		Since:    valueSince(e, kube.AttrExhausted),
		Summary: "Namespace quota is used up (" +
			strings.ReplaceAll(exhausted, ",", ", ") + ")",
	}}
}

// Attachment detects CSI volume attachments that failed.
type Attachment struct{}

// Name implements signal.Detector.
func (Attachment) Name() string { return "volume-attachment" }

// Kinds implements signal.Detector.
func (Attachment) Kinds() []knowledge.Kind {
	return []knowledge.Kind{kube.KindVolumeAttachment}
}

// Detect implements signal.Detector.
func (Attachment) Detect(
	_ signal.Context, e knowledge.Entity,
) []signal.Signal {
	message := text(e, kube.AttrAttachError)
	if message == "" {
		return nil
	}
	return []signal.Signal{{
		Reason:   constant.ReasonVolumeAttachmentFailure,
		Severity: signal.Critical,
		Since:    valueSince(e, kube.AttrAttachError),
		Summary:  "Volume cannot be attached to its node",
		Evidence: []signal.Evidence{{Label: "error", Value: message}},
	}}
}

// Webhook detects admission webhooks whose backend Service is missing or
// has no ready endpoints. With failurePolicy Fail this blocks every create
// or update the webhook intercepts.
type Webhook struct{}

// Name implements signal.Detector.
func (Webhook) Name() string { return "webhook" }

// Kinds implements signal.Detector.
func (Webhook) Kinds() []knowledge.Kind {
	return []knowledge.Kind{kube.KindMutatingWebhook, kube.KindValidatingHook}
}

// Detect implements signal.Detector.
func (Webhook) Detect(ctx signal.Context, e knowledge.Entity) []signal.Signal {
	blocking := strings.Contains(text(e, kube.AttrFailurePolicy), "Fail")
	severity := signal.Warning
	if blocking {
		severity = signal.Critical
	}
	for _, service := range ctx.Model.Related(
		e.ID, knowledge.Serves, knowledge.Outgoing,
	) {
		if !ctx.Synced(kube.KindService) {
			return nil
		}
		if !ctx.Model.Exists(service) {
			return []signal.Signal{{
				Reason:   constant.ReasonWebhookBackendNotFound,
				Severity: severity,
				Summary: "Admission webhook calls Service " + service.Name +
					", which does not exist",
			}}
		}
		if ready, known := readyEndpoints(ctx, service); known && ready == 0 {
			return []signal.Signal{{
				Reason: constant.ReasonWebhookNoEndpoints, Severity: severity,
				Summary: "Admission webhook backend " + service.Name +
					" has no ready pods",
			}}
		}
	}
	return nil
}

// readyEndpoints sums ready endpoints across a Service's slices.
func readyEndpoints(
	ctx signal.Context, service knowledge.EntityID,
) (float64, bool) {
	slices := ctx.Model.Related(service, knowledge.Backs, knowledge.Incoming)
	if len(slices) == 0 {
		return 0, false
	}
	ready := 0.0
	for _, id := range slices {
		if slice, ok := ctx.Model.Entity(id); ok {
			up, _ := number(slice, kube.AttrEndpointsReady)
			ready += up
		}
	}
	return ready, true
}
