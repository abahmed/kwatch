package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Virtual kinds of the control-plane components kwatch probes.
const (
	kindScheduler         inventory.Kind = "scheduler"
	kindControllerManager inventory.Kind = "controller-manager"
	kindEtcd              inventory.Kind = "etcd"
)

// controlPlaneRows cover the control plane (ADR 0010 rule 11): a
// stopped component shows up across the whole cluster at once. A
// stopped controller manager needs no row of its own: the generic
// controller-failing row links it to the workloads it stopped
// reconciling, through controlPlaneHops.
var controlPlaneRows = []Row{
	{
		// No scheduler: new pods wait without a node and without a
		// scheduling condition. One waiting pod is not a pattern.
		Name: "scheduler-unavailable",
		Cause: Side{Kind: kindScheduler,
			Modes: []detection.Mode{detection.ModeUnavailable}},
		Link:   LinkSchedules,
		Effect: podSide(detection.ModePending, detection.ModeUnschedulable),
		Prior:  0.75, MinCovered: 2,
	},
	{
		// The API server stores everything in etcd: etcd down is the
		// API server down. What stops behind the API server is the
		// apiserver-unavailable row.
		Name: "etcd-unavailable",
		Cause: Side{Kind: kindEtcd, Modes: []detection.Mode{
			detection.ModeUnavailable}},
		Link:   LinkServedBy,
		Effect: Side{Kind: kindAPIServer}, Prior: 0.8,
	},
}

// managedKinds are the built-in kinds the controller manager reconciles.
var managedKinds = map[inventory.Kind]bool{
	kube.KindDeployment: true, kube.KindReplicaSet: true,
	kube.KindStatefulSet: true, kube.KindDaemonSet: true,
	kube.KindJob: true, kube.KindCronJob: true, kube.KindHPA: true,
}

// controlPlaneHops lead from a failure to the control-plane component
// that would explain it. A hop depends only on the failure's own mode:
// the component is then an input of the failure's area, so the area is
// solved again when the component starts failing. A healthy component
// shows no mode and is never blamed.
func (v *view) controlPlaneHops(id inventory.EntityID) []hop {
	var out []hop
	if id.Kind == kube.KindPod &&
		v.hasMode(id, []detection.Mode{
			detection.ModePending, detection.ModeUnschedulable}) {
		out = append(out, hop{link: LinkSchedules, to: kube.Scheduler})
	}
	if id.Group == "" && managedKinds[id.Kind] &&
		v.hasMode(id, []detection.Mode{detection.ModeNotReconciling}) {
		out = append(out, hop{link: LinkManages,
			to: kube.ControllerManager})
	}
	if id == kube.APIServer && v.failing(id) {
		out = append(out, hop{link: LinkServedBy, to: kube.Etcd})
	}
	return out
}
