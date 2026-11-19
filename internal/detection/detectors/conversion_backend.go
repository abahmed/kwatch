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
// WebhookNoEndpoints), at Warning: they break the reads of one kind, not
// every create in the cluster.
func conversionBackendFindings(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	if ctx.Model == nil || !ctx.Synced(kube.KindService) {
		return nil
	}
	var out []detection.Finding
	for _, service := range ctx.Model.Related(
		e.ID, inventory.Serves, inventory.Outgoing,
	) {
		if !ctx.Model.Exists(service) {
			out = append(out, detection.Finding{
				Reason:   reasons.WebhookBackendNotFound,
				Severity: detection.Warning,
				Summary: "Conversion webhook calls Service " +
					service.Name + ", which does not exist",
				Evidence: []detection.Evidence{{
					Label: "service", Value: service.Name + " is gone"}},
			})
			continue
		}
		ready, known := readyEndpoints(ctx, service)
		if !known || ready > 0 || backendsAreStarting(ctx, service) {
			continue
		}
		out = append(out, detection.Finding{
			Reason: reasons.WebhookNoEndpoints, Severity: detection.Warning,
			Summary: "Conversion webhook backend " + service.Name +
				" has no ready pods",
			Evidence: []detection.Evidence{{
				Label: "service", Value: service.Name + " has no ready pods"}},
		})
	}
	return out
}
