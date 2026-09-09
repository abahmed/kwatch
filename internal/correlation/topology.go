package correlation

import (
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	corev1lister "k8s.io/client-go/listers/core/v1"

	"github.com/abahmed/kwatch/internal/model"
)

// dependentServices returns the Services whose selectors match the pod
// labels.
//
// The lister is supplied rather than read from the engine so this can run
// without e.mu held: matching a namespace of Services against one pod's
// labels is not work that needs to block every other producer.
func dependentServices(
	lister corev1lister.ServiceLister,
	namespace string,
	podLabels map[string]string,
) []string {
	if lister == nil || len(podLabels) == 0 {
		return nil
	}
	svcs, err := lister.Services(namespace).List(labels.Everything())
	if err != nil {
		return nil
	}
	var result []string
	for _, svc := range svcs {
		if len(svc.Spec.Selector) == 0 {
			continue
		}
		match := true
		for k, v := range svc.Spec.Selector {
			if podLabels[k] != v {
				match = false
				break
			}
		}
		if match {
			result = append(result, svc.Name)
		}
	}
	return result
}

// isOwnerHealthy reports whether the incident's owning workload is healthy,
// used to annotate pod incidents whose parent workload is also failing.
func (e *Engine) isOwnerHealthy(inc *model.Incident) bool {
	if inc.Resource != "pod" {
		return true
	}
	// The owning workload is a reference the incident carries. An incident
	// from an older on-disk state, or one built by hand, may not have it;
	// for a pod incident the subject reference names the same workload,
	// which is exactly what the key is built from.
	owner := inc.Owner
	if owner.Name == "" {
		owner = inc.Ref()
	}
	ns, name := owner.Namespace, owner.Name
	if ns == "" {
		ns = inc.Namespace
	}
	if ns == "" || name == "" {
		return true
	}

	switch inc.OwnerKind {
	case "Deployment":
		return e.deploymentHealthy(inc, ns, name)
	case "StatefulSet":
		return e.statefulSetHealthy(inc, ns, name)
	case "DaemonSet":
		return e.daemonSetHealthy(inc, ns, name)
	default:
		return true
	}
}

// ownerLookupHealthy decides how to gate an incident whose owning workload
// the lister did not return.
//
// A NotFound is an answer: the workload is deleted, so it cannot be the
// reason its former pods' incidents stay open, and holding them for a
// workload nobody will ever report on again means they only ever close as
// "not observed to recover". Any other error is not an answer, so an incident
// that still names pods of its own stays gated.
func ownerLookupHealthy(inc *model.Incident, err error) bool {
	if apierrors.IsNotFound(err) {
		return true
	}
	return len(inc.Resources) == 0
}

// A workload kwatch cannot read is treated as healthy only when the incident
// names no pods of its own: with pods still listed, the incident is about
// them and must not be gated on a workload nobody can see.
func (e *Engine) deploymentHealthy(
	inc *model.Incident, ns, name string,
) bool {
	if e.deployLister == nil {
		return true
	}
	d, err := e.deployLister.Deployments(ns).Get(name)
	if err != nil {
		return ownerLookupHealthy(inc, err)
	}
	if d.Status.ObservedGeneration < d.Generation {
		return false
	}
	return d.Status.ReadyReplicas >= d.Status.Replicas &&
		d.Status.UnavailableReplicas == 0
}

func (e *Engine) statefulSetHealthy(
	inc *model.Incident, ns, name string,
) bool {
	if e.ssLister == nil {
		return true
	}
	ss, err := e.ssLister.StatefulSets(ns).Get(name)
	if err != nil {
		return ownerLookupHealthy(inc, err)
	}
	if ss.Status.ObservedGeneration < ss.Generation {
		return false
	}
	return ss.Status.ReadyReplicas >= ss.Status.Replicas &&
		ss.Status.CurrentRevision == ss.Status.UpdateRevision
}

func (e *Engine) daemonSetHealthy(
	inc *model.Incident, ns, name string,
) bool {
	if e.dsLister == nil {
		return true
	}
	ds, err := e.dsLister.DaemonSets(ns).Get(name)
	if err != nil {
		return ownerLookupHealthy(inc, err)
	}
	return ds.Status.DesiredNumberScheduled > 0 &&
		ds.Status.NumberUnavailable == 0 &&
		ds.Status.UpdatedNumberScheduled == ds.Status.DesiredNumberScheduled
}
