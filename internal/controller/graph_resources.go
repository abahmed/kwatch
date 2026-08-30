package controller

import (
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/config"
	kwcontext "github.com/abahmed/kwatch/internal/context"
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
	graphEdgeUsesPull  = "uses_pull_secret"
	graphEdgeProjects  = "projects"   // pod → projected config/secret
	graphEdgeUsesCSI   = "uses_csi"   // pod → CSIDriver
	graphEdgeUsesSC    = "uses_sc"    // pvc/persistentvolume → storageclass
	graphEdgeLocalAt   = "local_at"   // persistentvolume → node (affinity)
	graphEdgeTLS       = "tls_secret" // ingress → TLS secret
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

// ownedByTargets describes edges from a resource to its workload owners,
// completing owner chains the pod informer cannot see directly
// (ReplicaSet→Deployment, Job→CronJob, ...).
func ownedByTargets(namespace string, ownerRefs []metav1.OwnerReference) []kwcontext.EdgeTarget {
	targets := make([]kwcontext.EdgeTarget, 0, len(ownerRefs))
	for _, ref := range ownerRefs {
		okind := strings.ToLower(ref.Kind)
		if !isTrackedWorkload(okind) {
			continue
		}
		targets = append(targets, kwcontext.EdgeTarget{
			Kind:      okind,
			Namespace: namespace,
			Name:      ref.Name,
			Type:      "owned_by",
		})
	}
	return targets
}

func isTrackedWorkload(k string) bool {
	switch k {
	case "deployment", "statefulset", "daemonset", "replicaset", "job", "cronjob":
		return true
	}
	return false
}

// rebuildFrom feeds a complete lister result into a per-object graph rebuild.
// A rebuild publishes its graph only after every required lister succeeds, so
// a cache failure must abort rather than replacing valid state with a partial
// graph.
func rebuildFrom[T any](items []T, err error, logMsg string, rebuild func(interface{})) error {
	if err != nil {
		return fmt.Errorf("%s: %w", logMsg, err)
	}
	for i := range items {
		rebuild(items[i])
	}
	return nil
}

func rebuildCheckedFrom[T any](items []T, err error, logMsg string, rebuild func(T) error) error {
	if err != nil {
		return fmt.Errorf("%s: %w", logMsg, err)
	}
	for _, item := range items {
		if err := rebuild(item); err != nil {
			return fmt.Errorf("%s: %w", logMsg, err)
		}
	}
	return nil
}

// buildResourceGraph rebuilds graph nodes for every non-pod resource type from
// the informer caches. Pod edges are rebuilt by buildGraph before this runs.
func (c *Controller) buildResourceGraph() error {
	if c.graph == nil {
		return nil
	}
	if c.pvcLister != nil {
		items, err := c.pvcLister.PersistentVolumeClaims(metav1.NamespaceAll).List(labels.Everything())
		if err := rebuildFrom(items, err, "list pvcs for graph build", c.rebuildPersistentVolumeClaim); err != nil {
			return err
		}
	}
	if c.rsLister != nil {
		items, err := c.rsLister.ReplicaSets(metav1.NamespaceAll).List(labels.Everything())
		if err := rebuildFrom(items, err, "list replicasets for graph build", c.rebuildReplicaSet); err != nil {
			return err
		}
	}
	if c.jobLister != nil {
		items, err := c.jobLister.Jobs(metav1.NamespaceAll).List(labels.Everything())
		if err := rebuildFrom(items, err, "list jobs for graph build", c.rebuildJob); err != nil {
			return err
		}
	}
	if err := c.buildServiceGraph(); err != nil {
		return err
	}
	if c.ingressLister != nil {
		items, err := c.ingressLister.Ingresses(metav1.NamespaceAll).List(labels.Everything())
		if err := rebuildFrom(items, err, "list ingresses for graph build", c.rebuildIngress); err != nil {
			return err
		}
	}
	if c.hpaLister != nil {
		items, err := c.hpaLister.HorizontalPodAutoscalers(metav1.NamespaceAll).List(labels.Everything())
		if err := rebuildFrom(items, err, "list hpas for graph build", c.rebuildHorizontalPodAutoscaler); err != nil {
			return err
		}
	}
	if c.netpolLister != nil {
		items, err := c.netpolLister.NetworkPolicies(metav1.NamespaceAll).List(labels.Everything())
		if err := rebuildCheckedFrom(items, err, "build networkpolicy graph edges", c.rebuildNetworkPolicyChecked); err != nil {
			return err
		}
	}
	if c.pdbLister != nil {
		items, err := c.pdbLister.PodDisruptionBudgets(metav1.NamespaceAll).List(labels.Everything())
		if err := rebuildCheckedFrom(items, err, "build poddisruptionbudget graph edges", c.rebuildPodDisruptionBudgetChecked); err != nil {
			return err
		}
	}
	if c.endpointSliceLister != nil {
		items, err := c.endpointSliceLister.EndpointSlices(metav1.NamespaceAll).List(labels.Everything())
		if err := rebuildFrom(items, err, "list endpoint slices for graph build", c.rebuildEndpointSlice); err != nil {
			return err
		}
	}
	if err := c.buildPersistentVolumeGraph(); err != nil {
		return err
	}
	return nil
}

func (c *Controller) buildServiceGraph() error {
	if c.serviceLister == nil {
		return nil
	}
	items, err := c.serviceLister.Services(metav1.NamespaceAll).List(labels.Everything())
	return rebuildCheckedFrom(items, err, "build service graph edges", c.rebuildServiceChecked)
}

func (c *Controller) buildPersistentVolumeGraph() error {
	if c.pvLister == nil {
		return nil
	}
	items, err := c.pvLister.List(labels.Everything())
	return rebuildCheckedFrom(items, err, "build persistentvolume graph edges", c.rebuildPersistentVolumeChecked)
}

// wireGraphHandlers attaches per-resource graph maintenance handlers to the
// shared informers for every resource type kwatch understands. Informers are
// lazily created by the shared factories, so re-requesting them here returns
// the same instances the process handlers use. Resources gated behind a
// monitor contribute edges only while that monitor is on; pod, configmap and
// the newly introduced PVC informer are always wired.
func (c *Controller) wireGraphHandlers(fs factorySet, cfg *config.Config) {
	if c.nodeLister != nil {
		inf := fs.nodeInformer()
		inf.AddEventHandler(c.graphHandler("node", c.rebuildNodeGraph))
	}
	if c.serviceLister != nil {
		for _, inf := range fs.serviceInformers() {
			inf.AddEventHandler(c.graphHandler("service", c.rebuildService))
		}
	}
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
