package controller

import (
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"
)

// rebuildNodeGraph refreshes PersistentVolume and node-heartbeat edges after
// a node changes. The PV refresh keeps node-local attachment relationships
// consistent with the informer snapshot.
func (c *Controller) rebuildNodeGraph(obj interface{}) {
	node, ok := obj.(*corev1.Node)
	if !ok || c.graph == nil {
		return
	}
	c.addNodeLeaseEdge(node.Name)
	if c.pvLister == nil {
		return
	}
	pvs, err := c.pvLister.List(labels.Everything())
	if err != nil {
		klog.ErrorS(err,
			"failed to refresh persistentvolume graph edges after node change",
			"node", node.Name)
		return
	}
	for _, pv := range pvs {
		if err := c.rebuildPersistentVolumeChecked(pv); err != nil {
			klog.ErrorS(err,
				"failed to refresh persistentvolume graph edges after node change",
				"node", node.Name, "pv", pv.Name)
		}
	}
}

func (c *Controller) addNodeLeaseEdge(nodeName string) {
	if nodeName == "" || c.leaseLister == nil {
		return
	}
	lease, err := c.leaseLister.Leases("kube-node-lease").Get(nodeName)
	if err != nil {
		return
	}
	c.graph.AddEdge(
		"node", "", nodeName, "lease", lease.Namespace,
		lease.Name, "heartbeat",
	)
}

func (c *Controller) rebuildLeaseGraph(obj interface{}) {
	lease, ok := obj.(*coordinationv1.Lease)
	if !ok || lease.Namespace != "kube-node-lease" || c.graph == nil {
		return
	}
	c.addNodeLeaseEdge(lease.Name)
}
