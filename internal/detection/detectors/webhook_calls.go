package detectors

import (
	"fmt"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Limits for one admission webhook's calls, read from the API server's
// metrics of them.
const (
	// slowHookMS: every create the webhook intercepts waits this long.
	slowHookMS = 2000.0
	// extremeHookMS is close to the API server's default 10s timeout,
	// where calls start to fail.
	extremeHookMS = 5000.0
	// failedHookPercent is the share of calls that fail.
	failedHookPercent = 20.0
	hookSlowSustain   = 5 * time.Minute
	// hookFailSustain is shorter: a webhook that fails closed refuses
	// requests while it fails.
	hookFailSustain = time.Minute
)

// WebhookCalls detects an admission webhook that is slow or that fails
// many calls, from how the API server's calls to it went. Webhook
// detects one that cannot be reached at all.
type WebhookCalls struct{}

// Name implements detection.Detector.
func (WebhookCalls) Name() string { return "webhook-calls" }

// Kinds implements detection.Detector.
func (WebhookCalls) Kinds() []inventory.Kind {
	return []inventory.Kind{kube.KindMutatingWebhook, kube.KindValidatingHook}
}

// Detect implements detection.Detector.
func (WebhookCalls) Detect(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	var out []detection.Finding
	out = append(out, slowWebhookFinding(ctx, e)...)
	return append(out, failingWebhook(ctx, e)...)
}

// slowWebhookFinding reports a webhook whose calls take seconds. It is
// information unless the calls are close to the API server's timeout.
func slowWebhookFinding(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	p99, ok := number(e, kube.AttrWebhookP99)
	if !ok || !held(ctx, "webhook-slow", p99, slowHookMS) {
		return nil
	}
	severity, wait := detection.Info, hookSlowSustain
	if p99 >= extremeHookMS {
		severity, wait = detection.Warning, extremeSustain
	}
	since := ctx.Onset("webhook-slow", ctx.Now)
	if !sustained(ctx, "webhook-slow", since, wait) {
		return nil
	}
	hook := text(e, kube.AttrWebhookSlowest)
	return []detection.Finding{{
		Reason: reasons.WebhookSlow, Severity: severity, Since: since,
		Summary: "Admission webhook " + hook + " takes " +
			millisText(p99) + " per call (p99); every request it " +
			"intercepts waits for it",
		Evidence: webhookEvidence(e),
	}}
}

// failingWebhook reports a webhook that fails a large share of calls. One
// that fails closed refuses those requests; one set to ignore failures
// lets them through unchecked.
func failingWebhook(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	hook := text(e, kube.AttrWebhookSlowest)
	if hook == "" {
		hook = e.ID.Name
	}
	if share, ok := number(e, kube.AttrWebhookClosedShare); ok &&
		held(ctx, "webhook-closed", share, failedHookPercent) {
		since := ctx.Onset("webhook-closed", ctx.Now)
		if sustained(ctx, "webhook-closed", since, hookFailSustain) {
			return []detection.Finding{{
				Reason: reasons.WebhookRejecting, Severity: detection.Warning,
				Since: since,
				Summary: fmt.Sprintf("Admission webhook %s fails %s of "+
					"its calls and fails closed: those requests are "+
					"refused", hook, percentText(share)),
				Evidence: webhookEvidence(e),
			}}
		}
		return nil
	}
	return failingOpen(ctx, e, hook)
}

func failingOpen(
	ctx detection.Context, e inventory.Entity, hook string,
) []detection.Finding {
	share, ok := number(e, kube.AttrWebhookOpenShare)
	if !ok || !held(ctx, "webhook-open", share, failedHookPercent) {
		return nil
	}
	since := ctx.Onset("webhook-open", ctx.Now)
	if !sustained(ctx, "webhook-open", since, hookSlowSustain) {
		return nil
	}
	return []detection.Finding{{
		Reason: reasons.WebhookFailingOpen, Severity: detection.Info,
		Since: since,
		Summary: fmt.Sprintf("Admission webhook %s fails %s of its "+
			"calls; it is set to ignore failures, so its checks are "+
			"skipped", hook, percentText(share)),
		Evidence: webhookEvidence(e),
	}}
}

// webhookEvidence is what the API server measured of the calls.
func webhookEvidence(e inventory.Entity) []detection.Evidence {
	var out []detection.Evidence
	if p99, ok := number(e, kube.AttrWebhookP99); ok {
		out = append(out, detection.Evidence{
			Label: "p99 per call", Value: millisText(p99)})
	}
	if calls, ok := number(e, kube.AttrWebhookCalls); ok {
		out = append(out, detection.Evidence{Label: "calls measured",
			Value: fmt.Sprintf("%.0f", calls)})
	}
	if policy := strings.TrimSpace(
		text(e, kube.AttrFailurePolicy)); policy != "" {
		out = append(out, detection.Evidence{
			Label: "failure policy", Value: policy})
	}
	return out
}
