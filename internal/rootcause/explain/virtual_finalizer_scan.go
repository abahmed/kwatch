package explain

import (
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// kindScan is the virtual kind of a scan input: "any entity of this
// kind". The handler of a finalizer is searched among every workload,
// role, binding and webhook of the cluster, so the entity that becomes
// the handler later is not known when the area is solved; the area
// records the kinds it scanned instead, and the incremental solver
// treats a dirty entity of one of them as a dirty input.
const kindScan inventory.Kind = "scan"

// scanOf is the scan input standing for every entity of kind.
func scanOf(kind inventory.Kind) inventory.EntityID {
	return inventory.CoreID(kindScan, "", string(kind))
}

// scannedKinds are the kinds the handler evidence reads.
var scannedKinds = []inventory.Kind{
	kube.KindDeployment, kube.KindStatefulSet, kube.KindDaemonSet,
	kube.KindRole, kube.KindClusterRole, kube.KindRoleBinding,
	kube.KindClusterRoleBinding, kube.KindValidatingHook,
	kube.KindMutatingWebhook, kube.KindCRD,
}

// gateHandlerScan records that the walk through a stuck object depends
// on every kind the handler evidence scans, so a handler that appears
// or changes later solves the area again.
func (v *view) gateHandlerScan(id inventory.EntityID) {
	for _, kind := range scannedKinds {
		v.gate(id, scanOf(kind))
	}
}
