package detectors

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// namespaceTerminating reports a namespace that is being deleted. Its
// ConfigMaps, Secrets and ServiceAccounts go before its pods get their
// deletion timestamp, so what the pods reference is gone by design.
func namespaceTerminating(ctx detection.Context, namespace string) bool {
	if namespace == "" || ctx.Model == nil {
		return false
	}
	ns, ok := ctx.Model.Entity(
		inventory.CoreID(kube.KindNamespace, "", namespace))
	return ok && (text(ns, kube.AttrPhase) == "Terminating" ||
		flag(ns, kube.AttrDeleting))
}
