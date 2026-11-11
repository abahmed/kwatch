package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
)

// replacedOwners are the controllers whose pods get new names when
// they are replaced, by the short name kubectl takes ("deploy/web").
// A StatefulSet is not one: its pods keep their names ("db-0"). A
// CronJob is not one either: kubectl cannot read its logs, and each
// run is a pod of its own Job.
var replacedOwners = map[inventory.Kind]string{
	kube.KindDeployment: "deploy",
	kube.KindReplicaSet: "rs",
	kube.KindDaemonSet:  "ds",
	kube.KindJob:        "job",
}

// podRef is what a step reads a pod through. A restart, rollout or
// eviction replaces a controller's pod under a new name, so a step
// written with the pod's name fails with NotFound minutes later; one
// written with its controller keeps working. owner is zero for a bare
// pod, a StatefulSet's pod and a pod whose controller is not known:
// those steps keep the pod's own name.
type podRef struct {
	pod, container, ns string
	owner              inventory.EntityID
}

// podRefOf finds the controller that runs a pod or container among
// known (the incident's root and impact). Pods are named after their
// controller, so the longest matching name wins.
func podRefOf(known []inventory.EntityID, id inventory.EntityID) podRef {
	pod, container := id.Name, ""
	if id.Kind == kube.KindContainer {
		pod, container = splitContainer(id.Name)
	}
	ref := podRef{pod: pod, container: container,
		ns: namespaceFlag(id.Namespace)}
	for _, w := range known {
		if replacedOwners[w.Kind] != "" && w.Namespace == id.Namespace &&
			strings.HasPrefix(pod, w.Name+"-") &&
			len(w.Name) > len(ref.owner.Name) {
			ref.owner = w
		}
	}
	return ref
}

// owned reports a pod read through its controller.
func (r podRef) owned() bool {
	return r.owner.Name != ""
}

// logs is the "kubectl logs" target: "deploy/web -c app", or the pod
// and container by name.
func (r podRef) logs() string {
	target := quote(r.pod)
	if r.owned() {
		target = replacedOwners[r.owner.Kind] + "/" + quote(r.owner.Name)
	}
	if r.container == "" {
		return target
	}
	return target + " -c " + quote(r.container)
}

// describe is the "kubectl describe" target: every pod named after the
// controller ("pods web-", kubectl matches the name prefix), so the
// replacement pod is described too, or the one pod by name.
func (r podRef) describe() string {
	if r.owned() {
		return "pods " + quote(r.owner.Name+"-")
	}
	return "pod " + quote(r.pod)
}

// podPattern is a grep -E pattern for the header and the pods named
// after the controller.
func (r podRef) podPattern() string {
	name := strings.ReplaceAll(r.owner.Name, ".", `\.`)
	return quote("^(NAME|POD|" + name + "-)")
}

// pickPodStep follows a "kubectl logs" step read through a controller:
// kubectl reads one pod of it, the most available one, which may not
// be the pod that fails. The list shows each pod's restarts, readiness
// and node, so the reader can name the one to read.
func (r podRef) pickPodStep() []notification.Step {
	if !r.owned() {
		return nil
	}
	return []notification.Step{{
		Text: "kubectl reads only one of " + r.owner.Name +
			"'s pods; to read another, find it in this list and use " +
			"its name in place of " + replacedOwners[r.owner.Kind] + "/" +
			r.owner.Name,
		Command: "kubectl get pods" + r.ns + " -o wide | grep -E " +
			r.podPattern(),
	}}
}

// usage is what "kubectl top pod " reads: one pod by name, or every
// pod of the namespace narrowed to the controller's.
func (r podRef) usage() string {
	if !r.owned() {
		return quote(r.pod) + r.ns + " --containers"
	}
	return strings.TrimPrefix(r.ns, " ") + " --containers | grep -E " +
		r.podPattern()
}
