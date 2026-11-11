package detectors

import (
	"slices"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// webhookKinds are the admission webhook configuration kinds.
var webhookKinds = []inventory.Kind{
	kube.KindMutatingWebhook, kube.KindValidatingHook,
}

// leftoverOf is the absent backend the leftovers of namespace share:
// of the Services an admission webhook calls in namespace, the first by
// name that does not exist or has nothing ready behind it, named by the
// controller workload whose pods it selects when there is one (see
// backendController). An operator
// that was removed or scaled to zero leaves its webhooks calling such a
// Service, and its expired certificates, parked Deployments and
// objects stuck on its finalizers in the same namespace. Naming one
// backend for all of them makes them one incident (incident/leftover.go)
// instead of one per object. Zero when no webhook backend in namespace
// is absent, or when Services are not synced.
func leftoverOf(
	ctx detection.Context, namespace string,
) inventory.EntityID {
	if ctx.Model == nil || namespace == "" ||
		!ctx.Synced(kube.KindService) {
		return inventory.EntityID{}
	}
	var found inventory.EntityID
	for _, kind := range webhookKinds {
		for _, config := range ctx.Model.Entities(kind) {
			for _, service := range ctx.Model.Related(
				config, inventory.Serves, inventory.Outgoing) {
				if service.Namespace != namespace ||
					(!found.IsZero() && service.Name >= found.Name) ||
					!backendAbsent(ctx, service) {
					continue
				}
				found = service
			}
		}
	}
	if found.IsZero() {
		return found
	}
	return backendController(ctx, found)
}

// controllerKinds are the workloads that may run an operator.
var controllerKinds = []inventory.Kind{
	kube.KindDeployment, kube.KindStatefulSet, kube.KindDaemonSet,
}

// backendController is the workload, the first by kind then name,
// whose pods service selects: the operator that was scaled to zero or
// stopped. The Service itself when no workload is left behind it.
func backendController(
	ctx detection.Context, service inventory.EntityID,
) inventory.EntityID {
	for _, kind := range controllerKinds {
		for _, id := range sortedIn(ctx, kind, service.Namespace) {
			e, ok := ctx.Model.Entity(id)
			if !ok {
				continue
			}
			for _, selected := range kube.ServicesSelecting(ctx.Model, e) {
				if selected == service {
					return id
				}
			}
		}
	}
	return service
}

// sortedIn lists the entities of kind in namespace by name.
func sortedIn(
	ctx detection.Context, kind inventory.Kind, namespace string,
) []inventory.EntityID {
	ids := slices.Clone(ctx.Model.EntitiesIn(kind, namespace))
	slices.SortFunc(ids, func(a, b inventory.EntityID) int {
		return strings.Compare(a.Name, b.Name)
	})
	return ids
}

// backendAbsent reports a webhook backend Service that does not exist
// or has no ready endpoint, by the same test as the webhook detector.
func backendAbsent(
	ctx detection.Context, service inventory.EntityID,
) bool {
	if !ctx.Model.Exists(service) {
		return true
	}
	ready, known := readyEndpoints(ctx, service)
	return known && ready == 0 && !backendsAreStarting(ctx, service)
}

// asLeftover marks f a leftover of the absent backend of its
// namespace, when there is one. Only a finding that is low on its own
// is marked: one that blocks or breaks something keeps its own
// incident and tier.
func asLeftover(
	ctx detection.Context, f detection.Finding, namespace string,
) detection.Finding {
	f.Leftover = leftoverOf(ctx, namespace)
	return f
}
