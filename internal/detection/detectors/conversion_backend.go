package detectors

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// conversionBackendFindings reports a CustomResourceDefinition whose
// conversion webhook calls a Service that does not exist or has no ready
// endpoint. Every read of a resource that needs converting between the
// stored and the requested version then fails, whatever the client is.
// The findings are the admission webhooks' (WebhookBackendNotFound,
// WebhookNoEndpoints), informational: a dead backend alone may never be
// called (an operator removed with its resources), so it is a digest
// line, dated from the cluster's record and grouped with what the same
// backend left behind. A client that fails on a conversion is its own
// failure, which the conversion-webhook-down row explains by this one.
func conversionBackendFindings(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	if ctx.Model == nil || !ctx.SyncedFor(kube.KindService,
		reasons.WebhookBackendNotFound, reasons.WebhookNoEndpoints) {
		return nil
	}
	var out []detection.Finding
	for _, service := range ctx.Model.Related(
		e.ID, inventory.Serves, inventory.Outgoing,
	) {
		if f, ok := conversionBackend(ctx, service); ok {
			out = append(out, f)
		}
	}
	return out
}

// conversionBackend is the finding for one Service a conversion webhook
// calls, when it is gone or has nothing ready behind it.
func conversionBackend(
	ctx detection.Context, service inventory.EntityID,
) (detection.Finding, bool) {
	f := detection.Finding{
		Reason: reasons.WebhookBackendNotFound, Severity: detection.Info,
		Summary: "Conversion webhook calls Service " + service.Name +
			", which does not exist",
		Evidence: []detection.Evidence{{
			Label: "service", Value: service.Name + " is gone"}},
	}
	if ctx.Model.Exists(service) {
		ready, known := readyEndpoints(ctx, service)
		if !known || ready > 0 || backendsAreStarting(ctx, service) {
			return detection.Finding{}, false
		}
		f.Reason = reasons.WebhookNoEndpoints
		f.Summary = "Conversion webhook backend " + service.Name +
			" has no ready pods"
		f.Evidence[0].Value = service.Name + " has no ready pods"
	}
	f.Since = backendDownSince(ctx, service)
	// Keyed like an admission webhook's leftover: by the workload
	// behind the Service, so one removed operator is one group.
	f.Leftover = backendController(ctx, service)
	return f, true
}
