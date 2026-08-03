package controller

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	admissionregistrationv1lister "k8s.io/client-go/listers/admissionregistration/v1"
	appsv1lister "k8s.io/client-go/listers/apps/v1"
	autoscalingv2lister "k8s.io/client-go/listers/autoscaling/v2"
	batchv1lister "k8s.io/client-go/listers/batch/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
	discoveryv1lister "k8s.io/client-go/listers/discovery/v1"
	networkingv1lister "k8s.io/client-go/listers/networking/v1"
	policyv1lister "k8s.io/client-go/listers/policy/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/config"
)

func resolveNamespaces(cfg *config.Config, clientset kubernetes.Interface) ([]string, error) {
	if cfg.NamespaceSelector != "" {
		list, err := clientset.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{
			LabelSelector: cfg.NamespaceSelector,
		})
		if err != nil {
			return nil, fmt.Errorf("namespaceSelector list failed: %w", err)
		}
		ns := make([]string, 0, len(list.Items))
		for _, n := range list.Items {
			ns = append(ns, n.Name)
		}
		return ns, nil
	}
	return cfg.AllowedNamespaces, nil
}

func newFactories(client kubernetes.Interface, namespaces []string, resync time.Duration) (factorySet, []informers.SharedInformerFactory) {
	if len(namespaces) <= 1 {
		var opts []informers.SharedInformerOption
		if len(namespaces) == 1 {
			opts = append(opts, informers.WithNamespace(namespaces[0]))
		} else {
			// Exclude kube-system from non-control-plane informers to reduce
			// memory and network overhead. The control-plane monitor uses a
			// dedicated kube-system-scoped factory.
			opts = append(opts, informers.WithTweakListOptions(func(o *metav1.ListOptions) {
				o.FieldSelector = "metadata.namespace!=kube-system"
			}))
		}
		factory := informers.NewSharedInformerFactoryWithOptions(client, resync, opts...)
		// Create a separate factory for cluster-scoped resources (Nodes,
		// MutatingWebhookConfigurations, ValidatingWebhookConfigurations) that
		// must NOT inherit the namespace field selector.
		clusterFactory := informers.NewSharedInformerFactoryWithOptions(client, resync)
		return factorySet{global: factory, clusterScoped: clusterFactory}, []informers.SharedInformerFactory{factory, clusterFactory}
	}

	factories := make([]informers.SharedInformerFactory, 0, len(namespaces))
	for _, ns := range namespaces {
		opts := []informers.SharedInformerOption{informers.WithNamespace(ns)}
		factories = append(factories, informers.NewSharedInformerFactoryWithOptions(client, resync, opts...))
	}
	return factorySet{perNamespace: factories}, factories
}

type factorySet struct {
	global        informers.SharedInformerFactory
	perNamespace  []informers.SharedInformerFactory
	clusterScoped informers.SharedInformerFactory
}

func (fs factorySet) podLister() corev1lister.PodLister {
	if fs.global != nil {
		return fs.global.Core().V1().Pods().Lister()
	}
	listers := make([]corev1lister.PodLister, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		listers = append(listers, f.Core().V1().Pods().Lister())
	}
	return &multiPodLister{listers: listers}
}

func (fs factorySet) podInformers() []cache.SharedIndexInformer {
	if fs.global != nil {
		return []cache.SharedIndexInformer{fs.global.Core().V1().Pods().Informer()}
	}
	out := make([]cache.SharedIndexInformer, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		out = append(out, f.Core().V1().Pods().Informer())
	}
	return out
}

func (fs factorySet) nodeLister() corev1lister.NodeLister {
	if fs.clusterScoped != nil {
		return fs.clusterScoped.Core().V1().Nodes().Lister()
	}
	if fs.global != nil {
		return fs.global.Core().V1().Nodes().Lister()
	}
	return fs.perNamespace[0].Core().V1().Nodes().Lister()
}

func (fs factorySet) nodeInformer() cache.SharedIndexInformer {
	if fs.clusterScoped != nil {
		return fs.clusterScoped.Core().V1().Nodes().Informer()
	}
	if fs.global != nil {
		return fs.global.Core().V1().Nodes().Informer()
	}
	return fs.perNamespace[0].Core().V1().Nodes().Informer()
}

func (fs factorySet) deployLister() appsv1lister.DeploymentLister {
	if fs.global != nil {
		return fs.global.Apps().V1().Deployments().Lister()
	}
	listers := make([]appsv1lister.DeploymentLister, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		listers = append(listers, f.Apps().V1().Deployments().Lister())
	}
	return &multiDeploymentLister{listers: listers}
}

func (fs factorySet) deployInformers() []cache.SharedIndexInformer {
	if fs.global != nil {
		return []cache.SharedIndexInformer{fs.global.Apps().V1().Deployments().Informer()}
	}
	out := make([]cache.SharedIndexInformer, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		out = append(out, f.Apps().V1().Deployments().Informer())
	}
	return out
}

func (fs factorySet) jobLister() batchv1lister.JobLister {
	if fs.global != nil {
		return fs.global.Batch().V1().Jobs().Lister()
	}
	listers := make([]batchv1lister.JobLister, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		listers = append(listers, f.Batch().V1().Jobs().Lister())
	}
	return &multiJobLister{listers: listers}
}

func (fs factorySet) rsLister() appsv1lister.ReplicaSetLister {
	if fs.global != nil {
		return fs.global.Apps().V1().ReplicaSets().Lister()
	}
	listers := make([]appsv1lister.ReplicaSetLister, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		listers = append(listers, f.Apps().V1().ReplicaSets().Lister())
	}
	return &multiReplicaSetLister{listers: listers}
}

func (fs factorySet) rsInformers() []cache.SharedIndexInformer {
	if fs.global != nil {
		return []cache.SharedIndexInformer{fs.global.Apps().V1().ReplicaSets().Informer()}
	}
	out := make([]cache.SharedIndexInformer, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		out = append(out, f.Apps().V1().ReplicaSets().Informer())
	}
	return out
}

func (fs factorySet) dsLister() appsv1lister.DaemonSetLister {
	if fs.global != nil {
		return fs.global.Apps().V1().DaemonSets().Lister()
	}
	listers := make([]appsv1lister.DaemonSetLister, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		listers = append(listers, f.Apps().V1().DaemonSets().Lister())
	}
	return &multiDaemonSetLister{listers: listers}
}

func (fs factorySet) dsInformers() []cache.SharedIndexInformer {
	if fs.global != nil {
		return []cache.SharedIndexInformer{fs.global.Apps().V1().DaemonSets().Informer()}
	}
	out := make([]cache.SharedIndexInformer, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		out = append(out, f.Apps().V1().DaemonSets().Informer())
	}
	return out
}

func (fs factorySet) ssLister() appsv1lister.StatefulSetLister {
	if fs.global != nil {
		return fs.global.Apps().V1().StatefulSets().Lister()
	}
	listers := make([]appsv1lister.StatefulSetLister, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		listers = append(listers, f.Apps().V1().StatefulSets().Lister())
	}
	return &multiStatefulSetLister{listers: listers}
}

func (fs factorySet) ssInformers() []cache.SharedIndexInformer {
	if fs.global != nil {
		return []cache.SharedIndexInformer{fs.global.Apps().V1().StatefulSets().Informer()}
	}
	out := make([]cache.SharedIndexInformer, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		out = append(out, f.Apps().V1().StatefulSets().Informer())
	}
	return out
}

func (fs factorySet) pdbLister() policyv1lister.PodDisruptionBudgetLister {
	if fs.global != nil {
		return fs.global.Policy().V1().PodDisruptionBudgets().Lister()
	}
	listers := make([]policyv1lister.PodDisruptionBudgetLister, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		listers = append(listers, f.Policy().V1().PodDisruptionBudgets().Lister())
	}
	return &multiPodDisruptionBudgetLister{listers: listers}
}

func (fs factorySet) pdbInformers() []cache.SharedIndexInformer {
	if fs.global != nil {
		return []cache.SharedIndexInformer{fs.global.Policy().V1().PodDisruptionBudgets().Informer()}
	}
	out := make([]cache.SharedIndexInformer, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		out = append(out, f.Policy().V1().PodDisruptionBudgets().Informer())
	}
	return out
}

func (fs factorySet) jobInformers() []cache.SharedIndexInformer {
	if fs.global != nil {
		return []cache.SharedIndexInformer{fs.global.Batch().V1().Jobs().Informer()}
	}
	out := make([]cache.SharedIndexInformer, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		out = append(out, f.Batch().V1().Jobs().Informer())
	}
	return out
}

func (fs factorySet) cronJobLister() batchv1lister.CronJobLister {
	if fs.global != nil {
		return fs.global.Batch().V1().CronJobs().Lister()
	}
	listers := make([]batchv1lister.CronJobLister, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		listers = append(listers, f.Batch().V1().CronJobs().Lister())
	}
	return &multiCronJobLister{listers: listers}
}

func (fs factorySet) cronJobInformers() []cache.SharedIndexInformer {
	if fs.global != nil {
		return []cache.SharedIndexInformer{fs.global.Batch().V1().CronJobs().Informer()}
	}
	out := make([]cache.SharedIndexInformer, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		out = append(out, f.Batch().V1().CronJobs().Informer())
	}
	return out
}

func (fs factorySet) hpaLister() autoscalingv2lister.HorizontalPodAutoscalerLister {
	if fs.global != nil {
		return fs.global.Autoscaling().V2().HorizontalPodAutoscalers().Lister()
	}
	listers := make([]autoscalingv2lister.HorizontalPodAutoscalerLister, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		listers = append(listers, f.Autoscaling().V2().HorizontalPodAutoscalers().Lister())
	}
	return &multiHorizontalPodAutoscalerLister{listers: listers}
}

func (fs factorySet) hpaInformers() []cache.SharedIndexInformer {
	if fs.global != nil {
		return []cache.SharedIndexInformer{fs.global.Autoscaling().V2().HorizontalPodAutoscalers().Informer()}
	}
	out := make([]cache.SharedIndexInformer, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		out = append(out, f.Autoscaling().V2().HorizontalPodAutoscalers().Informer())
	}
	return out
}

func (fs factorySet) serviceLister() corev1lister.ServiceLister {
	if fs.global != nil {
		return fs.global.Core().V1().Services().Lister()
	}
	listers := make([]corev1lister.ServiceLister, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		listers = append(listers, f.Core().V1().Services().Lister())
	}
	return &multiServiceLister{listers: listers}
}

func (fs factorySet) serviceInformers() []cache.SharedIndexInformer {
	if fs.global != nil {
		return []cache.SharedIndexInformer{fs.global.Core().V1().Services().Informer()}
	}
	out := make([]cache.SharedIndexInformer, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		out = append(out, f.Core().V1().Services().Informer())
	}
	return out
}

func (fs factorySet) endpointSliceLister() discoveryv1lister.EndpointSliceLister {
	if fs.global != nil {
		return fs.global.Discovery().V1().EndpointSlices().Lister()
	}
	listers := make([]discoveryv1lister.EndpointSliceLister, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		listers = append(listers, f.Discovery().V1().EndpointSlices().Lister())
	}
	return &multiEndpointSliceLister{listers: listers}
}

func (fs factorySet) endpointSliceInformers() []cache.SharedIndexInformer {
	if fs.global != nil {
		return []cache.SharedIndexInformer{fs.global.Discovery().V1().EndpointSlices().Informer()}
	}
	out := make([]cache.SharedIndexInformer, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		out = append(out, f.Discovery().V1().EndpointSlices().Informer())
	}
	return out
}

func (fs factorySet) ingressLister() networkingv1lister.IngressLister {
	if fs.global != nil {
		return fs.global.Networking().V1().Ingresses().Lister()
	}
	listers := make([]networkingv1lister.IngressLister, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		listers = append(listers, f.Networking().V1().Ingresses().Lister())
	}
	return &multiIngressLister{listers: listers}
}

func (fs factorySet) ingressInformers() []cache.SharedIndexInformer {
	if fs.global != nil {
		return []cache.SharedIndexInformer{fs.global.Networking().V1().Ingresses().Informer()}
	}
	out := make([]cache.SharedIndexInformer, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		out = append(out, f.Networking().V1().Ingresses().Informer())
	}
	return out
}

func (fs factorySet) netpolLister() networkingv1lister.NetworkPolicyLister {
	if fs.global != nil {
		return fs.global.Networking().V1().NetworkPolicies().Lister()
	}
	listers := make([]networkingv1lister.NetworkPolicyLister, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		listers = append(listers, f.Networking().V1().NetworkPolicies().Lister())
	}
	return &multiNetpolLister{listers: listers}
}

func (fs factorySet) netpolInformers() []cache.SharedIndexInformer {
	if fs.global != nil {
		return []cache.SharedIndexInformer{fs.global.Networking().V1().NetworkPolicies().Informer()}
	}
	out := make([]cache.SharedIndexInformer, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		out = append(out, f.Networking().V1().NetworkPolicies().Informer())
	}
	return out
}

func (fs factorySet) configMapLister() corev1lister.ConfigMapLister {
	if fs.global != nil {
		return fs.global.Core().V1().ConfigMaps().Lister()
	}
	listers := make([]corev1lister.ConfigMapLister, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		listers = append(listers, f.Core().V1().ConfigMaps().Lister())
	}
	return &multiConfigMapLister{listers: listers}
}

func (fs factorySet) configMapInformers() []cache.SharedIndexInformer {
	if fs.global != nil {
		return []cache.SharedIndexInformer{fs.global.Core().V1().ConfigMaps().Informer()}
	}
	out := make([]cache.SharedIndexInformer, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		out = append(out, f.Core().V1().ConfigMaps().Informer())
	}
	return out
}

func (fs factorySet) mwcLister() admissionregistrationv1lister.MutatingWebhookConfigurationLister {
	if fs.clusterScoped != nil {
		return fs.clusterScoped.Admissionregistration().V1().MutatingWebhookConfigurations().Lister()
	}
	if fs.global != nil {
		return fs.global.Admissionregistration().V1().MutatingWebhookConfigurations().Lister()
	}
	return fs.perNamespace[0].Admissionregistration().V1().MutatingWebhookConfigurations().Lister()
}

func (fs factorySet) mwcInformer() cache.SharedIndexInformer {
	if fs.clusterScoped != nil {
		return fs.clusterScoped.Admissionregistration().V1().MutatingWebhookConfigurations().Informer()
	}
	if fs.global != nil {
		return fs.global.Admissionregistration().V1().MutatingWebhookConfigurations().Informer()
	}
	return fs.perNamespace[0].Admissionregistration().V1().MutatingWebhookConfigurations().Informer()
}

func (fs factorySet) vwcLister() admissionregistrationv1lister.ValidatingWebhookConfigurationLister {
	if fs.clusterScoped != nil {
		return fs.clusterScoped.Admissionregistration().V1().ValidatingWebhookConfigurations().Lister()
	}
	if fs.global != nil {
		return fs.global.Admissionregistration().V1().ValidatingWebhookConfigurations().Lister()
	}
	return fs.perNamespace[0].Admissionregistration().V1().ValidatingWebhookConfigurations().Lister()
}

func (fs factorySet) vwcInformer() cache.SharedIndexInformer {
	if fs.clusterScoped != nil {
		return fs.clusterScoped.Admissionregistration().V1().ValidatingWebhookConfigurations().Informer()
	}
	if fs.global != nil {
		return fs.global.Admissionregistration().V1().ValidatingWebhookConfigurations().Informer()
	}
	return fs.perNamespace[0].Admissionregistration().V1().ValidatingWebhookConfigurations().Informer()
}
