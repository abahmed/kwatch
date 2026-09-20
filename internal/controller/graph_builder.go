package controller

import (
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"

	appsv1lister "k8s.io/client-go/listers/apps/v1"
	autoscalingv2lister "k8s.io/client-go/listers/autoscaling/v2"
	batchv1lister "k8s.io/client-go/listers/batch/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
	discoveryv1lister "k8s.io/client-go/listers/discovery/v1"
	networkingv1lister "k8s.io/client-go/listers/networking/v1"
	policyv1lister "k8s.io/client-go/listers/policy/v1"

	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
)

// graphBuilder contains only the informer-backed inputs required to construct
// a dependency graph. It deliberately does not copy Controller, which also
// owns queues, lifecycle state, and synchronization diagnostics.
type graphBuilder struct {
	graph               *kwcontext.ResourceGraph
	podLister           corev1lister.PodLister
	nodeLister          corev1lister.NodeLister
	pvcLister           corev1lister.PersistentVolumeClaimLister
	pvLister            corev1lister.PersistentVolumeLister
	rsLister            appsv1lister.ReplicaSetLister
	jobLister           batchv1lister.JobLister
	serviceLister       corev1lister.ServiceLister
	ingressLister       networkingv1lister.IngressLister
	hpaLister           autoscalingv2lister.HorizontalPodAutoscalerLister
	netpolLister        networkingv1lister.NetworkPolicyLister
	pdbLister           policyv1lister.PodDisruptionBudgetLister
	endpointSliceLister discoveryv1lister.EndpointSliceLister
}

// Controller wrappers keep informer callbacks stable while graph rebuilding
// uses the smaller graphBuilder dependency set.
func (c *Controller) rebuildService(obj interface{}) {
	c.newGraphBuilder(c.graph).rebuildService(obj)
}

func (c *Controller) rebuildServiceChecked(svc *corev1.Service) error {
	return c.newGraphBuilder(c.graph).rebuildServiceChecked(svc)
}

func (c *Controller) rebuildReplicaSet(obj interface{}) {
	c.newGraphBuilder(c.graph).rebuildReplicaSet(obj)
}

func (c *Controller) rebuildJob(obj interface{}) {
	c.newGraphBuilder(c.graph).rebuildJob(obj)
}

func (c *Controller) rebuildIngress(obj interface{}) {
	c.newGraphBuilder(c.graph).rebuildIngress(obj)
}

func (c *Controller) rebuildHorizontalPodAutoscaler(obj interface{}) {
	c.newGraphBuilder(c.graph).rebuildHorizontalPodAutoscaler(obj)
}

func (c *Controller) rebuildNetworkPolicy(obj interface{}) {
	c.newGraphBuilder(c.graph).rebuildNetworkPolicy(obj)
}

func (c *Controller) rebuildNetworkPolicyChecked(
	np *networkingv1.NetworkPolicy,
) error {
	return c.newGraphBuilder(c.graph).rebuildNetworkPolicyChecked(np)
}

func (c *Controller) rebuildPodDisruptionBudget(obj interface{}) {
	c.newGraphBuilder(c.graph).rebuildPodDisruptionBudget(obj)
}

func (c *Controller) rebuildPodDisruptionBudgetChecked(
	pdb *policyv1.PodDisruptionBudget,
) error {
	return c.newGraphBuilder(c.graph).rebuildPodDisruptionBudgetChecked(pdb)
}

func (c *Controller) rebuildEndpointSlice(obj interface{}) {
	c.newGraphBuilder(c.graph).rebuildEndpointSlice(obj)
}

func (c *Controller) rebuildPersistentVolumeClaim(obj interface{}) {
	c.newGraphBuilder(c.graph).rebuildPersistentVolumeClaim(obj)
}

func (c *Controller) rebuildPersistentVolume(obj interface{}) {
	c.newGraphBuilder(c.graph).rebuildPersistentVolume(obj)
}

func (c *Controller) rebuildPersistentVolumeChecked(
	pv *corev1.PersistentVolume,
) error {
	return c.newGraphBuilder(c.graph).rebuildPersistentVolumeChecked(pv)
}

func (c *Controller) addPodVolumeToGraph(
	ns, podName string, volume corev1.Volume,
) {
	c.newGraphBuilder(c.graph).addPodVolumeToGraph(ns, podName, volume)
}
