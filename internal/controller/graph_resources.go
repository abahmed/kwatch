package controller

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
)

// Edge types added by the per-resource graph builders. Keep them in sync with
// the string literals used in addPodToGraph (owned_by, scheduled_on, mounts,
// env_from, env_ref, selects).
const (
	graphEdgeRoutesTo  = "routes_to"  // ingress → service backend
	graphEdgeScales    = "scales"     // horizontalpodautoscaler → workload
	graphEdgeProtects  = "protects"   // poddisruptionbudget / networkpolicy → pod
	graphEdgeAppliesTo = "applies_to" // networkpolicy → pod
	graphEdgeBinds     = "binds"      // pvc → persistentvolume
	graphEdgeBacks     = "backs"      // endpointslice → service
	graphEdgeTargets   = "targets"    // endpointslice → pod
	graphEdgeUsesSA    = "uses_sa"    // pod → serviceaccount
	graphEdgeUsesSC    = "uses_sc"    // pvc → storageclass
	graphEdgeLocalAt   = "local_at"   // persistentvolume → node (affinity)
)

// graphHandler returns an event handler that maintains a resource's node in
// the dependency graph: add/update rebuild the node's outgoing edges, delete
// removes the node and any edges referencing it. kind must match the node kind
// used when building edges (lowercased).
func (c *Controller) graphHandler(kind string, rebuild func(interface{})) cache.ResourceEventHandlerFuncs {
	return cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { rebuild(obj) },
		UpdateFunc: func(_, newObj interface{}) { rebuild(newObj) },
		DeleteFunc: func(obj interface{}) {
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err != nil {
				return
			}
			ns, name, _ := cache.SplitMetaNamespaceKey(key)
			if c.graph != nil {
				c.graph.RemoveNode(kind, ns, name)
			}
		},
	}
}

// addOwnedByEdges records an owned_by edge from a resource to each of its
// workload owners, completing the owner chains the pod informer cannot see
// directly (ReplicaSet→Deployment, Job→CronJob, ...).
func (c *Controller) addOwnedByEdges(kind, ns, name string, ownerRefs []metav1.OwnerReference) {
	if c.graph == nil {
		return
	}
	for _, ref := range ownerRefs {
		okind := strings.ToLower(ref.Kind)
		if !isTrackedWorkload(okind) {
			continue
		}
		c.graph.AddEdge(kind, ns, name, okind, ns, ref.Name, "owned_by")
	}
}

func isTrackedWorkload(k string) bool {
	switch k {
	case "deployment", "statefulset", "daemonset", "replicaset", "job", "cronjob":
		return true
	}
	return false
}

// rebuildFrom feeds listed objects into a per-object graph rebuild, logging
// list failures instead of aborting the remaining resource types.
func rebuildFrom[T any](items []T, err error, logMsg string, rebuild func(interface{})) {
	if err != nil {
		klog.ErrorS(err, logMsg)
		return
	}
	for i := range items {
		rebuild(items[i])
	}
}

// buildResourceGraph rebuilds graph nodes for every non-pod resource type from
// the informer caches. Pod edges are rebuilt by buildGraph before this runs.
func (c *Controller) buildResourceGraph() {
	if c.graph == nil {
		return
	}
	if c.pvcLister != nil {
		items, err := c.pvcLister.PersistentVolumeClaims(metav1.NamespaceAll).List(labels.Everything())
		rebuildFrom(items, err, "failed to list pvcs for graph build", c.rebuildPersistentVolumeClaim)
	}
	if c.rsLister != nil {
		items, err := c.rsLister.ReplicaSets(metav1.NamespaceAll).List(labels.Everything())
		rebuildFrom(items, err, "failed to list replicasets for graph build", c.rebuildReplicaSet)
	}
	if c.jobLister != nil {
		items, err := c.jobLister.Jobs(metav1.NamespaceAll).List(labels.Everything())
		rebuildFrom(items, err, "failed to list jobs for graph build", c.rebuildJob)
	}
	if c.ingressLister != nil {
		items, err := c.ingressLister.Ingresses(metav1.NamespaceAll).List(labels.Everything())
		rebuildFrom(items, err, "failed to list ingresses for graph build", c.rebuildIngress)
	}
	if c.hpaLister != nil {
		items, err := c.hpaLister.HorizontalPodAutoscalers(metav1.NamespaceAll).List(labels.Everything())
		rebuildFrom(items, err, "failed to list hpas for graph build", c.rebuildHorizontalPodAutoscaler)
	}
	if c.netpolLister != nil {
		items, err := c.netpolLister.NetworkPolicies(metav1.NamespaceAll).List(labels.Everything())
		rebuildFrom(items, err, "failed to list networkpolicies for graph build", c.rebuildNetworkPolicy)
	}
	if c.pdbLister != nil {
		items, err := c.pdbLister.PodDisruptionBudgets(metav1.NamespaceAll).List(labels.Everything())
		rebuildFrom(items, err, "failed to list pdbs for graph build", c.rebuildPodDisruptionBudget)
	}
	if c.endpointSliceLister != nil {
		items, err := c.endpointSliceLister.EndpointSlices(metav1.NamespaceAll).List(labels.Everything())
		rebuildFrom(items, err, "failed to list endpoint slices for graph build", c.rebuildEndpointSlice)
	}
	if c.pvLister != nil {
		items, err := c.pvLister.List(labels.Everything())
		rebuildFrom(items, err, "failed to list persistentvolumes for graph build", c.rebuildPersistentVolume)
	}
}

// wireGraphHandlers attaches per-resource graph maintenance handlers to the
// shared informers for every resource type kwatch understands. Informers are
// lazily created by the shared factories, so re-requesting them here returns
// the same instances the process handlers use. Resources gated behind a
// monitor contribute edges only while that monitor is on; pod, configmap and
// the newly introduced PVC informer are always wired.
func (c *Controller) wireGraphHandlers(fs factorySet, cfg *config.Config) {
	for _, inf := range fs.rsInformers() {
		inf.AddEventHandler(c.graphHandler("replicaset", func(obj interface{}) { c.rebuildReplicaSet(obj) }))
	}
	for _, inf := range fs.pvcInformers() {
		inf.AddEventHandler(c.graphHandler("pvc", c.rebuildPersistentVolumeClaim))
	}
	for _, inf := range fs.persistentVolumeInformers() {
		inf.AddEventHandler(c.graphHandler("persistentvolume", c.rebuildPersistentVolume))
	}
	for _, inf := range fs.storageClassInformers() {
		inf.AddEventHandler(c.graphHandler("storageclass", func(interface{}) {}))
	}

	if cfg.JobMonitor.Enabled {
		for _, inf := range fs.jobInformers() {
			inf.AddEventHandler(c.graphHandler("job", c.rebuildJob))
		}
	}
	if cfg.IngressMonitor.Enabled {
		for _, inf := range fs.ingressInformers() {
			inf.AddEventHandler(c.graphHandler("ingress", c.rebuildIngress))
		}
	}
	if cfg.HpaMonitor.Enabled {
		for _, inf := range fs.hpaInformers() {
			inf.AddEventHandler(c.graphHandler("horizontalpodautoscaler", c.rebuildHorizontalPodAutoscaler))
		}
	}
	if cfg.NetworkPolicyMonitor.Enabled {
		for _, inf := range fs.netpolInformers() {
			inf.AddEventHandler(c.graphHandler("networkpolicy", c.rebuildNetworkPolicy))
		}
	}
	if cfg.PdbMonitor.Enabled {
		for _, inf := range fs.pdbInformers() {
			inf.AddEventHandler(c.graphHandler("poddisruptionbudget", c.rebuildPodDisruptionBudget))
		}
	}
	if cfg.ServiceMonitor.Enabled {
		for _, inf := range fs.endpointSliceInformers() {
			inf.AddEventHandler(c.graphHandler("endpointslice", c.rebuildEndpointSlice))
		}
	}
}
