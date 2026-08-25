package correlation

import (
	"k8s.io/apimachinery/pkg/labels"

	"github.com/abahmed/kwatch/internal/model"
)

// findDependentServices returns the names of Services in the given namespace
// whose selectors match the provided pod labels. Returns nil if no service
// lister is configured or no matches are found.
func (e *Engine) findDependentServices(namespace string, podLabels map[string]string) []string {
	if e.serviceLister == nil || len(podLabels) == 0 {
		return nil
	}
	svcs, err := e.serviceLister.Services(namespace).List(labels.Everything())
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
	ns := inc.Namespace
	name := inc.Name
	if ns == "" || name == "" {
		return true
	}

	switch inc.OwnerKind {
	case "Deployment":
		if e.deployLister == nil {
			return true
		}
		d, err := e.deployLister.Deployments(ns).Get(name)
		if err != nil {
			return len(inc.Resources) == 0
		}
		if d.Status.ObservedGeneration < d.Generation {
			return false
		}
		return d.Status.ReadyReplicas >= d.Status.Replicas &&
			d.Status.UnavailableReplicas == 0

	case "StatefulSet":
		if e.ssLister == nil {
			return true
		}
		ss, err := e.ssLister.StatefulSets(ns).Get(name)
		if err != nil {
			return len(inc.Resources) == 0
		}
		if ss.Status.ObservedGeneration < ss.Generation {
			return false
		}
		return ss.Status.ReadyReplicas >= ss.Status.Replicas &&
			ss.Status.CurrentRevision == ss.Status.UpdateRevision

	case "DaemonSet":
		if e.dsLister == nil {
			return true
		}
		ds, err := e.dsLister.DaemonSets(ns).Get(name)
		if err != nil {
			return len(inc.Resources) == 0
		}
		return ds.Status.DesiredNumberScheduled > 0 &&
			ds.Status.NumberUnavailable == 0 &&
			ds.Status.UpdatedNumberScheduled == ds.Status.DesiredNumberScheduled

	default:
		return true
	}
}
