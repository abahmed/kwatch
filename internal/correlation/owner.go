package correlation

import (
	corev1 "k8s.io/api/core/v1"
	appsv1lister "k8s.io/client-go/listers/apps/v1"
	"k8s.io/klog/v2"
)

// ResolveOwnerName returns the workload owner name used in incident keys
// for the given pod. ReplicaSet → parent Deployment; DaemonSet/StatefulSet
// resolve to themselves (or their parent if one exists); no owner → pod name.
// On lister error returns "" — callers must treat that as
// "unresolved, do nothing" (never guess).
func ResolveOwnerName(
	pod *corev1.Pod,
	rs appsv1lister.ReplicaSetLister,
	ds appsv1lister.DaemonSetLister,
	ss appsv1lister.StatefulSetLister,
) string {
	if len(pod.OwnerReferences) == 0 {
		return pod.Name
	}
	owner := pod.OwnerReferences[0]
	switch owner.Kind {
	case "ReplicaSet":
		if rs == nil {
			return ""
		}
		r, err := rs.ReplicaSets(pod.Namespace).Get(owner.Name)
		if err != nil {
			klog.ErrorS(err, "owner resolve: ReplicaSet lister", "pod", pod.Name)
			return ""
		}
		if len(r.OwnerReferences) > 0 {
			return r.OwnerReferences[0].Name
		}
		return owner.Name
	case "DaemonSet":
		if ds == nil {
			return ""
		}
		d, err := ds.DaemonSets(pod.Namespace).Get(owner.Name)
		if err != nil {
			klog.ErrorS(err, "owner resolve: DaemonSet lister", "pod", pod.Name)
			return ""
		}
		if len(d.OwnerReferences) > 0 {
			return d.OwnerReferences[0].Name
		}
		return owner.Name
	case "StatefulSet":
		if ss == nil {
			return ""
		}
		s, err := ss.StatefulSets(pod.Namespace).Get(owner.Name)
		if err != nil {
			klog.ErrorS(err, "owner resolve: StatefulSet lister", "pod", pod.Name)
			return ""
		}
		if len(s.OwnerReferences) > 0 {
			return s.OwnerReferences[0].Name
		}
		return owner.Name
	default:
		return owner.Name
	}
}
