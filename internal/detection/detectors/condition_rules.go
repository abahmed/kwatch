package detectors

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// The condition table: which status condition of which kind is a
// finding, how long it must hold, and what the finding is. Detectors
// with more to judge than one condition (readiness, HPA, schedules,
// jobs, the generic condition list) keep their own code.

const kindCRD inventory.Kind = "customresourcedefinition"

// resizeDeferredGrace is how long a deferred resize may wait for room on
// the node before it is reported; the kubelet retries it.
const resizeDeferredGrace = 5 * time.Minute

func init() {
	registerNodeAndWorkload()
	registerClusterObjects()
}

func registerNodeAndWorkload() {
	addConditions([]inventory.Kind{kube.KindNode},
		nodePressureCheck("MemoryPressure", reasons.MemoryPressure,
			"Node is low on memory; pods may be evicted"),
		nodePressureCheck("DiskPressure", reasons.DiskPressure,
			"Node is low on disk space; pods may be evicted"),
		nodePressureCheck("PIDPressure", reasons.PIDPressure,
			"Node is running out of process IDs"),
		conditionCheck{cond: "NetworkUnavailable", status: "True",
			finding: reasons.NetworkUnavailable, severity: detection.Critical,
			summary: "Node network is not configured ({reason})"},
	)
	workloads := []inventory.Kind{
		kube.KindDeployment, kube.KindStatefulSet, kube.KindDaemonSet,
	}
	addConditions(workloads,
		rolloutStuckCheck(reasons.ProgressDeadlineExceeded),
		replicaFailureCheck(reasons.DeploymentReplicaFailure),
	)
	// A ReplicaSet's availability is its Deployment's; only its replica
	// failures (quota, admission) are its own.
	addConditions([]inventory.Kind{kube.KindReplicaSet},
		rolloutStuckCheck(reasons.ReplicaSetFailure),
		replicaFailureCheck(reasons.ReplicaSetFailure),
	)
}

func registerClusterObjects() {
	addConditions([]inventory.Kind{kube.KindPDB},
		conditionCheck{cond: "DisruptionAllowed", status: "False",
			reason: "SyncFailed", grace: DefaultCustomFailing,
			key: "pdb-sync-failed", finding: reasons.PdbSyncFailed,
			severity: detection.Warning, health: detection.Failing,
			summary: "Disruption controller cannot evaluate the budget; " +
				"it allows no evictions",
			message: quoteMessage},
	)
	addConditions([]inventory.Kind{kindCRD},
		crdCheck("NamesAccepted"), crdCheck("Established"),
	)
	// A claim reports one finding: the first resize failure.
	addConditions([]inventory.Kind{kube.KindPVC},
		resizeErrorCheck("ControllerResizeError"),
		resizeErrorCheck("NodeResizeError"),
		volumeModifyCheck(),
	)
	addConditions([]inventory.Kind{kube.KindPod},
		conditionCheck{cond: "PodResizePending", status: "True",
			reason: "Infeasible", finding: reasons.PodResizeInfeasible,
			severity: detection.Warning, message: quoteMessageIfAny,
			summary: "Pod resize cannot be done on its node"},
		// The kubelet retries a deferred resize, so it waits for room.
		conditionCheck{cond: "PodResizePending", status: "True",
			reason: "Deferred", grace: resizeDeferredGrace,
			key: "resize-deferred", finding: reasons.PodResizeDeferred,
			severity: detection.Warning, message: quoteMessageIfAny,
			summary: "Pod resize has been deferred for lack of room on " +
				"its node"},
		conditionCheck{cond: "PodResizeInProgress", status: "True",
			reason: "Error", finding: reasons.PodResizeError,
			severity: detection.Warning, message: quoteMessageIfAny,
			summary: "Pod resize failed to apply"},
	)
}

// nodePressureCheck is a node pressure condition that held for
// DefaultNodePressure.
func nodePressureCheck(cond, finding, summary string) conditionCheck {
	return conditionCheck{cond: cond, status: "True",
		grace: DefaultNodePressure, key: "pressure/" + cond,
		finding: finding, severity: detection.Critical, summary: summary}
}

func rolloutStuckCheck(finding string) conditionCheck {
	return conditionCheck{cond: "Progressing",
		reason: reasons.ProgressDeadlineExceeded, finding: finding,
		severity: detection.Critical,
		summary: "Rollout is stuck: new pods did not become available " +
			"within the progress deadline"}
}

func replicaFailureCheck(finding string) conditionCheck {
	return conditionCheck{cond: "ReplicaFailure", status: "True",
		finding: finding, severity: detection.Critical,
		summary: "Controller cannot create pods ({reason})",
		message: quoteMessage}
}

// crdCheck is a CRD whose names conflict or that is not yet Established:
// its custom resources cannot be created or read. Only the first of the
// two is reported.
func crdCheck(cond string) conditionCheck {
	return conditionCheck{cond: cond, status: "False",
		grace: DefaultCustomFailing, key: "crd/" + cond,
		finding: reasons.CRDNotEstablished, severity: detection.Warning,
		health: detection.Failing, group: "crd", message: quoteMessage,
		summary: "Custom resource API is not served: " +
			"{type}=False{reasonNote}"}
}

func resizeErrorCheck(cond string) conditionCheck {
	return conditionCheck{cond: cond, status: "True", group: "resize",
		finding: reasons.PersistentVolumeClaim, severity: detection.Warning,
		summary: "Volume resize failed ({reason})", message: quoteMessage}
}

// volumeModifyCheck is a claim whose volume attributes class could not
// be applied (a failed VolumeAttributesClass modification). It shares
// the resize group: a claim reports one volume failure.
func volumeModifyCheck() conditionCheck {
	return conditionCheck{cond: "ModifyVolumeError", status: "True",
		group: "resize", finding: reasons.PersistentVolumeClaim,
		severity: detection.Warning, message: quoteMessage,
		summary: "Volume modification failed ({reason})"}
}
