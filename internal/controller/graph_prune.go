package controller

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"
)

// pruneGraph performs mark-and-sweep on the resource graph: removes
// ConfigMap, Secret, and Service nodes that no longer exist in the
// informer cache. This prevents stale entries from accumulating
// between full rebuilds.
// markActiveKeys records the graph node keys of a listed resource kind so
// dependent edges can be pruned against them.
func markActiveKeys[T metav1.Object](
	active map[string]bool,
	kind string,
	items []T,
	err error,
	logMsg string,
) bool {
	if err != nil {
		klog.ErrorS(err, logMsg)
		return false
	}
	for _, obj := range items {
		active[kind+"/"+obj.GetNamespace()+"/"+obj.GetName()] = true
	}
	return true
}

func (c *Controller) pruneGraph() {
	if c.graph == nil {
		return
	}

	active := make(map[string]bool)
	listed := map[string]bool{
		"configmap":        c.listActiveConfigMaps(active),
		"secret":           c.listActiveSecrets(active),
		"service":          c.listActiveServices(active),
		"serviceaccount":   c.listActiveServiceAccounts(active),
		"storageclass":     c.listActiveStorageClasses(active),
		"persistentvolume": c.listActivePersistentVolumes(active),
		"pvc":              c.listActivePersistentVolumeClaims(active),
	}

	pre := len(c.graph.Edges())
	for kind, success := range listed {
		if success {
			c.graph.Prune(kind, active)
		}
	}
	post := len(c.graph.Edges())
	if pruned := pre - post; pruned > 0 {
		klog.V(4).InfoS("graph pruned", "removed", pruned, "remaining", post)
	}
}

func (c *Controller) listActiveConfigMaps(active map[string]bool) bool {
	if c.configMapLister == nil {
		return false
	}
	items, err := c.configMapLister.List(labels.Everything())
	return markActiveKeys(
		active, "configmap", items, err,
		"failed to list configmaps for graph pruning",
	)
}

func (c *Controller) listActiveSecrets(active map[string]bool) bool {
	if c.secretLister == nil {
		return false
	}
	items, err := c.secretLister.List(labels.Everything())
	return markActiveKeys(
		active, "secret", items, err,
		"failed to list secrets for graph pruning",
	)
}

func (c *Controller) listActiveServices(active map[string]bool) bool {
	if c.serviceLister == nil {
		return false
	}
	items, err := c.serviceLister.List(labels.Everything())
	return markActiveKeys(
		active, "service", items, err,
		"failed to list services for graph pruning",
	)
}

func (c *Controller) listActiveServiceAccounts(active map[string]bool) bool {
	if c.serviceAccountLister == nil {
		return false
	}
	items, err := c.serviceAccountLister.List(labels.Everything())
	return markActiveKeys(
		active, "serviceaccount", items, err,
		"failed to list serviceaccounts for graph pruning",
	)
}

func (c *Controller) listActiveStorageClasses(active map[string]bool) bool {
	if c.storageClassLister == nil {
		return false
	}
	items, err := c.storageClassLister.List(labels.Everything())
	return markActiveKeys(
		active, "storageclass", items, err,
		"failed to list storageclasses for graph pruning",
	)
}

func (c *Controller) listActivePersistentVolumes(active map[string]bool) bool {
	if c.pvLister == nil {
		return false
	}
	items, err := c.pvLister.List(labels.Everything())
	return markActiveKeys(
		active, "persistentvolume", items, err,
		"failed to list persistentvolumes for graph pruning",
	)
}

func (c *Controller) listActivePersistentVolumeClaims(
	active map[string]bool,
) bool {
	if c.pvcLister == nil {
		return false
	}
	items, err := c.pvcLister.List(labels.Everything())
	return markActiveKeys(
		active, "pvc", items, err,
		"failed to list pvcs for graph pruning",
	)
}
