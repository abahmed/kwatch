package controller

import (
	"fmt"
	"strings"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"

	kwcontext "github.com/abahmed/kwatch/internal/context"
)

func (c *Controller) addPodToGraph(pod *corev1.Pod) {
	if err := c.addPodToGraphChecked(pod); err != nil {
		klog.ErrorS(err, "failed to build pod graph edges", "namespace", pod.Namespace, "pod", pod.Name)
	}
}

func (c *Controller) addPodToGraphChecked(pod *corev1.Pod) error {
	if c.graph == nil {
		return nil
	}
	ns := pod.Namespace
	name := pod.Name

	if pod.Spec.NodeName != "" {
		c.graph.AddEdge("pod", ns, name, "node", "", pod.Spec.NodeName, "scheduled_on")
	}
	if pod.Spec.ServiceAccountName != "" {
		c.graph.AddEdge("pod", ns, name, "serviceaccount", ns, pod.Spec.ServiceAccountName, "uses_sa")
	}
	for _, secret := range pod.Spec.ImagePullSecrets {
		if secret.Name != "" {
			c.graph.AddEdge("pod", ns, name, "secret", ns, secret.Name, graphEdgeUsesPull)
		}
	}
	for _, ref := range pod.OwnerReferences {
		ownerKind := strings.ToLower(ref.Kind)
		c.graph.AddEdge("pod", ns, name, ownerKind, ns, ref.Name, "owned_by")
	}
	for _, vol := range pod.Spec.Volumes {
		c.addPodVolumeToGraph(ns, name, vol)
	}
	for _, ctr := range pod.Spec.Containers {
		c.addContainerEnvToGraph(ns, name, ctr)
	}
	for _, ctr := range pod.Spec.InitContainers {
		c.addContainerEnvToGraph(ns, name, ctr)
	}

	if c.serviceLister == nil {
		return nil
	}
	svcs, err := c.serviceLister.Services(ns).List(labels.Everything())
	if err != nil {
		return fmt.Errorf("list services for graph edge: %w", err)
	}
	for _, svc := range svcs {
		if svc.Spec.Selector == nil {
			continue
		}
		if labels.SelectorFromSet(svc.Spec.Selector).Matches(labels.Set(pod.Labels)) {
			c.graph.AddEdge("service", ns, svc.Name, "pod", ns, name, "selects")
		}
	}
	return nil
}

func (c *Controller) addPodVolumeToGraph(ns, podName string, vol corev1.Volume) {
	if cm := vol.ConfigMap; cm != nil {
		c.graph.AddEdge("pod", ns, podName, "configmap", ns, cm.Name, "mounts")
	}
	if secret := vol.Secret; secret != nil {
		c.graph.AddEdge("pod", ns, podName, "secret", ns, secret.SecretName, "mounts")
	}
	if pvc := vol.PersistentVolumeClaim; pvc != nil {
		c.graph.AddEdge("pod", ns, podName, "pvc", ns, pvc.ClaimName, "mounts")
	}
	if projected := vol.Projected; projected != nil {
		for _, source := range projected.Sources {
			if source.ConfigMap != nil {
				c.graph.AddEdge("pod", ns, podName, "configmap", ns, source.ConfigMap.Name, graphEdgeProjects)
			}
			if source.Secret != nil {
				c.graph.AddEdge("pod", ns, podName, "secret", ns, source.Secret.Name, graphEdgeProjects)
			}
		}
	}
	if csi := vol.CSI; csi != nil && csi.Driver != "" {
		c.graph.AddEdge("pod", ns, podName, "csidriver", "", csi.Driver, graphEdgeUsesCSI)
	}
}

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
		klog.ErrorS(err, "failed to refresh persistentvolume graph edges after node change", "node", node.Name)
		return
	}
	for _, pv := range pvs {
		if err := c.rebuildPersistentVolumeChecked(pv); err != nil {
			klog.ErrorS(err, "failed to refresh persistentvolume graph edges after node change", "node", node.Name, "pv", pv.Name)
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
	c.graph.AddEdge("node", "", nodeName, "lease", lease.Namespace, lease.Name, "heartbeat")
}

func (c *Controller) rebuildLeaseGraph(obj interface{}) {
	lease, ok := obj.(*coordinationv1.Lease)
	if !ok || lease.Namespace != "kube-node-lease" || c.graph == nil {
		return
	}
	c.addNodeLeaseEdge(lease.Name)
}

func (c *Controller) removePodFromGraph(namespace, name string) {
	if c.graph == nil {
		return
	}
	c.graph.RemoveNode("pod", namespace, name)
}

func (c *Controller) rebuildPodGraph(pod *corev1.Pod) {
	if c.graph == nil {
		return
	}

	next := *c
	next.graph = kwcontext.NewResourceGraph()
	if err := next.addPodToGraphChecked(pod); err != nil {
		klog.ErrorS(err, "failed to rebuild pod graph edges; keeping previous edges", "namespace", pod.Namespace, "pod", pod.Name)
		return
	}

	podKey := "pod/" + pod.Namespace + "/" + pod.Name
	c.graph.ReplaceMatchingEdges(func(edge kwcontext.Edge) bool {
		return edge.From == podKey || (edge.To == podKey && edge.Type == "selects")
	}, next.graph.Edges())
	c.refreshPodSelectorEdges(pod.Namespace)
}

// refreshPodSelectorEdges keeps selector-based relationships accurate when a
// pod is created or its labels change. These resources point at pods, so their
// outgoing edges cannot be repaired by rebuilding the pod alone.
func (c *Controller) refreshPodSelectorEdges(namespace string) {
	if c.netpolLister != nil {
		policies, err := c.netpolLister.NetworkPolicies(namespace).List(labels.Everything())
		if err != nil {
			klog.ErrorS(err, "failed to refresh networkpolicy graph edges", "namespace", namespace)
		} else {
			for _, policy := range policies {
				if err := c.rebuildNetworkPolicyChecked(policy); err != nil {
					klog.ErrorS(err, "failed to refresh networkpolicy graph edges", "namespace", policy.Namespace, "name", policy.Name)
				}
			}
		}
	}
	if c.pdbLister != nil {
		budgets, err := c.pdbLister.PodDisruptionBudgets(namespace).List(labels.Everything())
		if err != nil {
			klog.ErrorS(err, "failed to refresh poddisruptionbudget graph edges", "namespace", namespace)
		} else {
			for _, budget := range budgets {
				if err := c.rebuildPodDisruptionBudgetChecked(budget); err != nil {
					klog.ErrorS(err, "failed to refresh poddisruptionbudget graph edges", "namespace", budget.Namespace, "name", budget.Name)
				}
			}
		}
	}
}

func (c *Controller) addContainerEnvToGraph(ns, podName string, ctr corev1.Container) {
	for _, envFrom := range ctr.EnvFrom {
		if cm := envFrom.ConfigMapRef; cm != nil {
			c.graph.AddEdge("pod", ns, podName, "configmap", ns, cm.Name, "env_from")
		}
		if s := envFrom.SecretRef; s != nil {
			c.graph.AddEdge("pod", ns, podName, "secret", ns, s.Name, "env_from")
		}
	}
	for _, env := range ctr.Env {
		if env.ValueFrom != nil {
			if cm := env.ValueFrom.ConfigMapKeyRef; cm != nil {
				c.graph.AddEdge("pod", ns, podName, "configmap", ns, cm.Name, "env_ref")
			}
			if s := env.ValueFrom.SecretKeyRef; s != nil {
				c.graph.AddEdge("pod", ns, podName, "secret", ns, s.Name, "env_ref")
			}
		}
	}
}

func (c *Controller) buildGraph() {
	if c.graph == nil {
		return
	}

	next := *c
	next.graph = kwcontext.NewResourceGraph()
	if err := next.buildGraphContents(); err != nil {
		klog.ErrorS(err, "failed to rebuild dependency graph; keeping previous graph")
		return
	}
	c.graph.ReplaceWith(next.graph)
	klog.V(4).InfoS("dependency graph built from informer cache", "edges", len(c.graph.Edges()))
}

func (c *Controller) buildGraphContents() error {
	pods, err := c.podLister.List(labels.Everything())
	if err != nil {
		return fmt.Errorf("list pods for graph build: %w", err)
	}
	for _, pod := range pods {
		if err := c.addPodToGraphChecked(pod); err != nil {
			return fmt.Errorf("build graph edges for pod %s/%s: %w", pod.Namespace, pod.Name, err)
		}
	}
	return c.buildResourceGraph()
}

// pruneGraph performs mark-and-sweep on the resource graph: removes
// ConfigMap, Secret, and Service nodes that no longer exist in the
// informer cache. This prevents stale entries from accumulating
// between full rebuilds.
// markActiveKeys records the graph node keys of a listed resource kind so
// dependent edges can be pruned against them.
func markActiveKeys[T metav1.Object](active map[string]bool, kind string, items []T, err error, logMsg string) bool {
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
	listed := make(map[string]bool)

	if cmLister := c.configMapLister; cmLister != nil {
		cms, err := cmLister.List(labels.Everything())
		listed["configmap"] = markActiveKeys(active, "configmap", cms, err, "failed to list configmaps for graph pruning")
	}

	if secretLister := c.secretLister; secretLister != nil {
		secrets, err := secretLister.List(labels.Everything())
		listed["secret"] = markActiveKeys(active, "secret", secrets, err, "failed to list secrets for graph pruning")
	}

	if svcLister := c.serviceLister; svcLister != nil {
		svcs, err := svcLister.List(labels.Everything())
		listed["service"] = markActiveKeys(active, "service", svcs, err, "failed to list services for graph pruning")
	}

	if saLister := c.serviceAccountLister; saLister != nil {
		sas, err := saLister.List(labels.Everything())
		listed["serviceaccount"] = markActiveKeys(active, "serviceaccount", sas, err, "failed to list serviceaccounts for graph pruning")
	}

	if scLister := c.storageClassLister; scLister != nil {
		scs, err := scLister.List(labels.Everything())
		listed["storageclass"] = markActiveKeys(active, "storageclass", scs, err, "failed to list storageclasses for graph pruning")
	}

	if pvLister := c.pvLister; pvLister != nil {
		pvs, err := pvLister.List(labels.Everything())
		listed["persistentvolume"] = markActiveKeys(active, "persistentvolume", pvs, err, "failed to list persistentvolumes for graph pruning")
	}

	if pvcLister := c.pvcLister; pvcLister != nil {
		pvcs, err := pvcLister.List(labels.Everything())
		listed["pvc"] = markActiveKeys(active, "pvc", pvcs, err, "failed to list pvcs for graph pruning")
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
