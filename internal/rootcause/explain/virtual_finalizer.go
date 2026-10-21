package explain

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// LinkFinalizes is the link from what removes a finalizer to the object it
// holds. The cause is what removes the finalizer that holds the
// effect's deletion: the controller that handles it, or, when none is
// known, the finalizer itself.
const LinkFinalizes LinkType = "finalizes"

// KindFinalizer is the virtual entity of one finalizer that holds
// deletions in one namespace. Its name is the finalizer's.
const KindFinalizer inventory.Kind = "finalizer"

// Pseudo modes of a finalizer's handler and of the finalizer itself.
const (
	// ModeHandlerStopped is a controller that has no running replica.
	// Its finer modes say why.
	ModeHandlerStopped detection.Mode = "HandlerStopped"
	// ModeHandlerScaledDown: the controller is scaled to zero.
	ModeHandlerScaledDown = ModeHandlerStopped + ".ScaledToZero"
	// ModeHandlerNotReady: it wants replicas and none is ready.
	ModeHandlerNotReady = ModeHandlerStopped + ".NotReady"
	// ModeFinalizerUnhandled is a finalizer nothing removes. The finer
	// mode ".Running" says a controller known to handle it runs, so
	// kwatch cannot say it is down.
	ModeFinalizerUnhandled detection.Mode = "FinalizerUnhandled"
	// ModeFinalizerHandlerRuns is ModeFinalizerUnhandled with a
	// controller that runs.
	ModeFinalizerHandlerRuns = ModeFinalizerUnhandled + ".Running"
)

// handlerKinds are the workloads that may run a finalizer's controller.
var handlerKinds = []inventory.Kind{
	kube.KindDeployment, kube.KindStatefulSet, kube.KindDaemonSet,
}

// finalizerHops lead from an object whose deletion is stuck to what
// should remove its finalizers: the controller scaled down or not
// ready, else the finalizer itself.
func (v *view) finalizerHops(id inventory.EntityID) []hop {
	names := v.finalizersOf(id)
	if len(names) == 0 {
		return nil
	}
	v.gateHandlerScan(id)
	handlers := v.handlersOf(id)
	var out []hop
	for _, handler := range handlers {
		v.gate(id, handler)
		if v.handlerStopped(handler) != "" {
			out = append(out, hop{link: LinkFinalizes, to: handler})
		}
	}
	if len(out) > 0 {
		return out
	}
	for _, name := range names {
		out = append(out, hop{link: LinkFinalizes, to: inventory.CoreID(
			KindFinalizer, id.Namespace, name)})
	}
	return out
}

// finalizerModes are the pseudo modes of a finalizer's handler and of
// the finalizer, read from the stuck object they are the cause of. ok
// is false for any other entity or link.
func (v *view) finalizerModes(
	id, effect inventory.EntityID, link LinkType,
) (modes []modeHealth, ok bool) {
	if link != LinkFinalizes && link != LinkManages {
		return nil, false
	}
	names := v.finalizersOf(effect)
	if len(names) == 0 {
		return nil, false
	}
	failing := func(mode detection.Mode) ([]modeHealth, bool) {
		return pseudoFailing(mode), true
	}
	switch {
	case id.Kind == KindFinalizer && link == LinkFinalizes:
		if id.Namespace != effect.Namespace || !hasText(names, id.Name) {
			return nil, false
		}
		if len(v.handlersOf(effect)) > 0 {
			return failing(ModeFinalizerHandlerRuns)
		}
		return failing(ModeFinalizerUnhandled)
	case containsID(v.handlersOf(effect), id):
		if mode := v.handlerStopped(id); mode != "" {
			return failing(mode)
		}
	}
	return nil, false
}

// finalizersOf lists the finalizers that hold the deletion of id, as its
// StuckDeleting finding names them. Finalizers Kubernetes itself runs
// are left out: their handler is the control plane, not a workload.
func (v *view) finalizersOf(id inventory.EntityID) []string {
	if id.Kind == kube.KindNamespace || id.Kind == kube.KindPod ||
		id.Kind == kube.KindNode {
		return nil
	}
	var out []string
	for _, f := range v.s.Findings[id] {
		if f.Mode == detection.ModeStuckDeleting {
			out = append(out, findingFinalizers(f)...)
		}
	}
	return out
}

// findingFinalizers are the finalizers a finding names that a workload
// may run.
func findingFinalizers(f detection.Finding) []string {
	var out []string
	for _, e := range f.Evidence {
		if e.Label != detection.EvidenceFinalizers {
			continue
		}
		for _, name := range strings.Split(e.Value, ", ") {
			if !builtinFinalizer(name) {
				out = append(out, name)
			}
		}
	}
	return out
}

// builtinFinalizer reports a finalizer that Kubernetes itself removes:
// the bare names ("foregroundDeletion") and the kubernetes.io and k8s.io
// domains.
func builtinFinalizer(name string) bool {
	domain, _, ok := strings.Cut(name, "/")
	return !ok || builtinDomain(domain)
}

func builtinDomain(domain string) bool {
	for _, suffix := range []string{"kubernetes.io", "k8s.io"} {
		if domain == suffix || strings.HasSuffix(domain, "."+suffix) {
			return true
		}
	}
	return false
}

func hasText(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

// handlerStopped says why a controller workload runs nothing: scaled to
// zero, or wanting replicas with none ready. Empty when it runs, or
// when its replica counts are not known.
func (v *view) handlerStopped(id inventory.EntityID) detection.Mode {
	e, ok := v.s.Model.Entity(id)
	if !ok {
		return ""
	}
	want, known := attrNumber(e, kube.AttrReplicas)
	if !known {
		want, known = attrNumber(e, kube.AttrDesiredReplicas)
	}
	ready, readyKnown := attrNumber(e, kube.AttrReadyReplicas)
	switch {
	case !known:
		return ""
	case want == 0:
		return ModeHandlerScaledDown
	case readyKnown && ready == 0:
		return ModeHandlerNotReady
	}
	return ""
}

// handlersOf are the workloads known to run the controller of the
// finalizers holding id. Objects held by the same finalizers in one
// namespace are one problem, so the answer is the one of the whole
// group (see sharedHandlers). A workload is named only with positive
// evidence that it serves the finalizer's API group, checked in this
// order, the first that finds any wins (see groupHandlers):
//
//  1. the field manager that owns the finalizer entry itself;
//  2. a pod label or annotation key in the group's domain, or container
//     arguments or environment quoting the group or the finalizer;
//  3. a ServiceAccount bound to a role that writes the group;
//  4. a webhook that admits, or a conversion webhook of, the group.
//
// A name alone is never evidence; see finalizerHint.
func (v *view) handlersOf(id inventory.EntityID) []inventory.EntityID {
	return v.sharedHandlers(id)
}

func sortedIDs(ids []inventory.EntityID) []inventory.EntityID {
	sortIDs(ids)
	return ids
}

// holdsFinalizers reports a candidate chosen for objects whose deletion
// a finalizer holds. They are leftovers, not workloads that fail
// together, so the "workloads meet here" evidence does not apply.
func holdsFinalizers(c *candidate) bool {
	name := c.bestMatch().match.row.Name
	return name == "finalizer-unhandled" ||
		name == "finalizer-handler-stopped"
}
