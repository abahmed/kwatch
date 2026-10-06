package detectors

import (
	"slices"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// DefaultBudgetBlocked is how long a PDB may block every disruption before
// it matters: drains and upgrades wait on it.
const DefaultBudgetBlocked = 10 * time.Minute

// DefaultBudgetTooStrict is how long a budget that can never allow an
// eviction may block a drain. Every pod it expects is healthy and still
// none may go, so minAvailable equals the replica count: no replacement
// is on its way and waiting cannot help. The minute only absorbs the lag
// of the budget's status after the cordon.
const DefaultBudgetTooStrict = time.Minute

// Budget detects PodDisruptionBudgets that block all disruptions because
// the pods they protect are unhealthy.
type Budget struct{}

// Name implements detection.Detector.
func (Budget) Name() string { return "disruption-budget" }

// Kinds implements detection.Detector.
func (Budget) Kinds() []inventory.Kind { return []inventory.Kind{kube.KindPDB} }

// Detect implements detection.Detector.
func (Budget) Detect(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	out := budgetMisconfiguration(ctx, e)
	if f, ok := budgetBlocking(ctx, e); ok {
		out = append(out, f)
	}
	if f, ok := budgetBlocksDrain(ctx, e); ok {
		out = append(out, f)
	}
	return out
}

func budgetBlocking(
	ctx detection.Context, e inventory.Entity,
) (detection.Finding, bool) {
	allowed, _ := number(e, kube.AttrDisruptionsAllowed)
	healthy, _ := number(e, kube.AttrCurrentHealthy)
	desired, _ := number(e, kube.AttrDesiredHealthy)
	expected, _ := number(e, kube.AttrExpectedPods)
	if allowed > 0 || expected == 0 || healthy >= desired {
		return detection.Finding{}, false
	}
	since := valueSince(e, kube.AttrDisruptionsAllowed)
	if !sustained(ctx, "pdb-blocked", since, DefaultBudgetBlocked) {
		return detection.Finding{}, false
	}
	return detection.Finding{
		Reason: reasons.PdbViolation, Severity: detection.Warning,
		Since: since,
		Summary: "Disruption budget blocks node drains: too few healthy " +
			"pods",
	}, true
}

// Quota detects ResourceQuotas with an exhausted resource, or resources
// close to their hard limit.
type Quota struct{}

// Name implements detection.Detector.
func (Quota) Name() string { return "quota" }

// Kinds implements detection.Detector.
func (Quota) Kinds() []inventory.Kind {
	return []inventory.Kind{kube.KindQuota}
}

// Detect implements detection.Detector.
func (Quota) Detect(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	exhausted := withoutZeroHard(
		text(e, kube.AttrExhausted), text(e, kube.AttrQuotaZeroHard))
	if exhausted == "" {
		return quotaNearLimit(e)
	}
	// Used equal to hard is a full quota, not yet a failure: it is a
	// warning only once a controller was refused for exceeding it.
	severity := detection.Info
	if quotaRefusedCreate(ctx, e.ID.Namespace) {
		severity = detection.Warning
	}
	return []detection.Finding{{
		Reason:   reasons.ResourceQuotaExhausted,
		Severity: severity,
		Since:    valueSince(e, kube.AttrExhausted),
		Summary: "Namespace quota is used up (" +
			strings.ReplaceAll(exhausted, ",", ", ") + ")",
	}}
}

// withoutZeroHard drops the resources whose hard limit is zero from the
// exhausted list. A zero limit means "none allowed", not "ran out".
func withoutZeroHard(exhausted, zeroHard string) string {
	if zeroHard == "" {
		return exhausted
	}
	skip := strings.Split(zeroHard, ",")
	var kept []string
	for _, name := range strings.Split(exhausted, ",") {
		if name != "" && !slices.Contains(skip, name) {
			kept = append(kept, name)
		}
	}
	return strings.Join(kept, ",")
}

// Attachment detects CSI volume attachments that failed to attach or
// cannot detach.
type Attachment struct{}

// Name implements detection.Detector.
func (Attachment) Name() string { return "volume-attachment" }

// Kinds implements detection.Detector.
func (Attachment) Kinds() []inventory.Kind {
	return []inventory.Kind{kube.KindVolumeAttachment}
}

// Detect implements detection.Detector.
func (Attachment) Detect(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	out := detachFailure(ctx, e)
	message := text(e, kube.AttrAttachError)
	since := valueSince(e, kube.AttrAttachError)
	// Any failed attempt sets the error, and a successful retry clears
	// it, so only an error that lasts is a problem.
	if message == "" ||
		!sustained(ctx, "attach-error", since, DefaultCustomFailing) {
		return out
	}
	return append(out, detection.Finding{
		Reason:   reasons.VolumeAttachmentFailure,
		Severity: detection.Critical,
		Since:    since,
		Summary:  "Volume cannot be attached to its node",
		Evidence: []detection.Evidence{{Label: "error", Value: message}},
	})
}

// Webhook detects admission webhooks whose backend Service is missing or
// has no ready endpoints. With failurePolicy Fail this blocks every create
// or update the webhook intercepts.
type Webhook struct{}

// Name implements detection.Detector.
func (Webhook) Name() string { return "webhook" }

// Kinds implements detection.Detector.
func (Webhook) Kinds() []inventory.Kind {
	return []inventory.Kind{kube.KindMutatingWebhook, kube.KindValidatingHook}
}

// Detect implements detection.Detector.
func (Webhook) Detect(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	if ctx.Model == nil {
		return nil
	}
	blocking := strings.Contains(text(e, kube.AttrFailurePolicy), "Fail")
	severity := detection.Warning
	if blocking {
		severity = detection.Critical
	}
	for _, service := range ctx.Model.Related(
		e.ID, inventory.Serves, inventory.Outgoing,
	) {
		if !ctx.Synced(kube.KindService) {
			return nil
		}
		if !ctx.Model.Exists(service) {
			return []detection.Finding{{
				Reason:   reasons.WebhookBackendNotFound,
				Severity: severity,
				Summary: "Admission webhook calls Service " + service.Name +
					", which does not exist",
			}}
		}
		if ready, known := readyEndpoints(ctx, service); known && ready == 0 {
			if backendsAreStarting(ctx, service) {
				return nil
			}
			return []detection.Finding{{
				Reason: reasons.WebhookNoEndpoints, Severity: severity,
				Summary: "Admission webhook backend " + service.Name +
					" has no ready pods",
			}}
		}
	}
	return nil
}

// readyEndpoints sums ready endpoints across a Service's slices.
func readyEndpoints(
	ctx detection.Context, service inventory.EntityID,
) (float64, bool) {
	slices := ctx.Model.Related(service, inventory.Backs, inventory.Incoming)
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
