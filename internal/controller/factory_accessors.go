package controller

import (
	appsv1lister "k8s.io/client-go/listers/apps/v1"
	autoscalingv2lister "k8s.io/client-go/listers/autoscaling/v2"
	batchv1lister "k8s.io/client-go/listers/batch/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
	discoveryv1lister "k8s.io/client-go/listers/discovery/v1"
	policyv1lister "k8s.io/client-go/listers/policy/v1"
	"k8s.io/client-go/tools/cache"
)

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
