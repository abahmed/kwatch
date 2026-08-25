package controller

import (
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	appsv1lister "k8s.io/client-go/listers/apps/v1"
	autoscalingv2lister "k8s.io/client-go/listers/autoscaling/v2"
	batchv1lister "k8s.io/client-go/listers/batch/v1"
	discoveryv1lister "k8s.io/client-go/listers/discovery/v1"
	networkingv1lister "k8s.io/client-go/listers/networking/v1"
	policyv1lister "k8s.io/client-go/listers/policy/v1"
)

// CronJob
// ---------------------------------------------------------------------------

type multiCronJobLister struct {
	listers []batchv1lister.CronJobLister
}

func (m *multiCronJobLister) List(selector labels.Selector) ([]*batchv1.CronJob, error) {
	return listAll(selector, m.listers)
}

func (m *multiCronJobLister) CronJobs(namespace string) batchv1lister.CronJobNamespaceLister {
	return &multiNamespace[*batchv1.CronJob, batchv1lister.CronJobNamespaceLister]{
		listers: nsAll(m.listers, namespace, batchv1lister.CronJobLister.CronJobs),
		gr:      schema.GroupResource{Group: "batch", Resource: "cronjobs"},
	}
}

// ---------------------------------------------------------------------------
// DaemonSet
// ---------------------------------------------------------------------------

type multiDaemonSetLister struct {
	listers []appsv1lister.DaemonSetLister
}

func (m *multiDaemonSetLister) List(selector labels.Selector) ([]*appsv1.DaemonSet, error) {
	return listAll(selector, m.listers)
}

func (m *multiDaemonSetLister) DaemonSets(namespace string) appsv1lister.DaemonSetNamespaceLister {
	return &multiNamespace[*appsv1.DaemonSet, appsv1lister.DaemonSetNamespaceLister]{
		listers: nsAll(m.listers, namespace, appsv1lister.DaemonSetLister.DaemonSets),
		gr:      schema.GroupResource{Group: "apps", Resource: "daemonsets"},
	}
}

func (m *multiDaemonSetLister) GetPodDaemonSets(pod *corev1.Pod) ([]*appsv1.DaemonSet, error) {
	for _, l := range m.listers {
		dl, ok := interface{}(l).(interface {
			GetPodDaemonSets(*corev1.Pod) ([]*appsv1.DaemonSet, error)
		})
		if ok {
			dss, err := dl.GetPodDaemonSets(pod)
			if err == nil {
				return dss, nil
			}
		}
	}
	return nil, fmt.Errorf("no daemonsets found for pod %s/%s", pod.Namespace, pod.Name)
}

func (m *multiDaemonSetLister) GetHistoryDaemonSets(history *appsv1.ControllerRevision) ([]*appsv1.DaemonSet, error) {
	for _, l := range m.listers {
		dl, ok := interface{}(l).(interface {
			GetHistoryDaemonSets(*appsv1.ControllerRevision) ([]*appsv1.DaemonSet, error)
		})
		if ok {
			dss, err := dl.GetHistoryDaemonSets(history)
			if err == nil {
				return dss, nil
			}
		}
	}
	return nil, fmt.Errorf("no daemonsets found for history %s/%s", history.Namespace, history.Name)
}

// ---------------------------------------------------------------------------
// Deployment
// ---------------------------------------------------------------------------

type multiDeploymentLister struct {
	listers []appsv1lister.DeploymentLister
}

func (m *multiDeploymentLister) List(selector labels.Selector) ([]*appsv1.Deployment, error) {
	return listAll(selector, m.listers)
}

func (m *multiDeploymentLister) Deployments(namespace string) appsv1lister.DeploymentNamespaceLister {
	return &multiNamespace[*appsv1.Deployment, appsv1lister.DeploymentNamespaceLister]{
		listers: nsAll(m.listers, namespace, appsv1lister.DeploymentLister.Deployments),
		gr:      schema.GroupResource{Group: "apps", Resource: "deployments"},
	}
}

// ---------------------------------------------------------------------------
// EndpointSlice
// ---------------------------------------------------------------------------

type multiEndpointSliceLister struct {
	listers []discoveryv1lister.EndpointSliceLister
}

func (m *multiEndpointSliceLister) List(selector labels.Selector) ([]*discoveryv1.EndpointSlice, error) {
	return listAll(selector, m.listers)
}

func (m *multiEndpointSliceLister) EndpointSlices(namespace string) discoveryv1lister.EndpointSliceNamespaceLister {
	return &multiNamespace[*discoveryv1.EndpointSlice, discoveryv1lister.EndpointSliceNamespaceLister]{
		listers: nsAll(m.listers, namespace, discoveryv1lister.EndpointSliceLister.EndpointSlices),
		gr:      schema.GroupResource{Group: "", Resource: "endpointslices"},
	}
}

// ---------------------------------------------------------------------------
// HorizontalPodAutoscaler
// ---------------------------------------------------------------------------

type multiHorizontalPodAutoscalerLister struct {
	listers []autoscalingv2lister.HorizontalPodAutoscalerLister
}

func (m *multiHorizontalPodAutoscalerLister) List(selector labels.Selector) ([]*autoscalingv2.HorizontalPodAutoscaler, error) {
	return listAll(selector, m.listers)
}

func (m *multiHorizontalPodAutoscalerLister) HorizontalPodAutoscalers(namespace string) autoscalingv2lister.HorizontalPodAutoscalerNamespaceLister {
	return &multiNamespace[*autoscalingv2.HorizontalPodAutoscaler, autoscalingv2lister.HorizontalPodAutoscalerNamespaceLister]{
		listers: nsAll(m.listers, namespace, autoscalingv2lister.HorizontalPodAutoscalerLister.HorizontalPodAutoscalers),
		gr:      schema.GroupResource{Group: "autoscaling", Resource: "horizontalpodautoscalers"},
	}
}

// ---------------------------------------------------------------------------
// Ingress
// ---------------------------------------------------------------------------

type multiIngressLister struct {
	listers []networkingv1lister.IngressLister
}

func (m *multiIngressLister) List(selector labels.Selector) ([]*networkingv1.Ingress, error) {
	return listAll(selector, m.listers)
}

func (m *multiIngressLister) Ingresses(namespace string) networkingv1lister.IngressNamespaceLister {
	return &multiNamespace[*networkingv1.Ingress, networkingv1lister.IngressNamespaceLister]{
		listers: nsAll(m.listers, namespace, networkingv1lister.IngressLister.Ingresses),
		gr:      schema.GroupResource{Group: "networking.k8s.io", Resource: "ingresses"},
	}
}

// ---------------------------------------------------------------------------
// Job
// ---------------------------------------------------------------------------

type multiJobLister struct {
	listers []batchv1lister.JobLister
}

func (m *multiJobLister) List(selector labels.Selector) ([]*batchv1.Job, error) {
	return listAll(selector, m.listers)
}

func (m *multiJobLister) Jobs(namespace string) batchv1lister.JobNamespaceLister {
	return &multiNamespace[*batchv1.Job, batchv1lister.JobNamespaceLister]{
		listers: nsAll(m.listers, namespace, batchv1lister.JobLister.Jobs),
		gr:      schema.GroupResource{Group: "batch", Resource: "jobs"},
	}
}

func (m *multiJobLister) GetPodJobs(pod *corev1.Pod) ([]batchv1.Job, error) {
	for _, l := range m.listers {
		jobs, err := l.GetPodJobs(pod)
		if err == nil {
			return jobs, nil
		}
	}
	return nil, fmt.Errorf("no jobs found for pod %s/%s", pod.Namespace, pod.Name)
}

// ---------------------------------------------------------------------------
// MutatingWebhookConfiguration / ValidatingWebhookConfiguration
// are cluster-scoped only and need no multi-lister.
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// NetworkPolicy
// ---------------------------------------------------------------------------

type multiNetpolLister struct {
	listers []networkingv1lister.NetworkPolicyLister
}

func (m *multiNetpolLister) List(selector labels.Selector) ([]*networkingv1.NetworkPolicy, error) {
	return listAll(selector, m.listers)
}

func (m *multiNetpolLister) NetworkPolicies(namespace string) networkingv1lister.NetworkPolicyNamespaceLister {
	return &multiNamespace[*networkingv1.NetworkPolicy, networkingv1lister.NetworkPolicyNamespaceLister]{
		listers: nsAll(m.listers, namespace, networkingv1lister.NetworkPolicyLister.NetworkPolicies),
		gr:      schema.GroupResource{Group: "networking.k8s.io", Resource: "networkpolicies"},
	}
}

// ---------------------------------------------------------------------------
// PodDisruptionBudget
// ---------------------------------------------------------------------------

type multiPodDisruptionBudgetLister struct {
	listers []policyv1lister.PodDisruptionBudgetLister
}

func (m *multiPodDisruptionBudgetLister) List(selector labels.Selector) ([]*policyv1.PodDisruptionBudget, error) {
	return listAll(selector, m.listers)
}

func (m *multiPodDisruptionBudgetLister) PodDisruptionBudgets(namespace string) policyv1lister.PodDisruptionBudgetNamespaceLister {
	return &multiNamespace[*policyv1.PodDisruptionBudget, policyv1lister.PodDisruptionBudgetNamespaceLister]{
		listers: nsAll(m.listers, namespace, policyv1lister.PodDisruptionBudgetLister.PodDisruptionBudgets),
		gr:      schema.GroupResource{Group: "policy", Resource: "poddisruptionbudgets"},
	}
}

func (m *multiPodDisruptionBudgetLister) GetPodPodDisruptionBudgets(pod *corev1.Pod) ([]*policyv1.PodDisruptionBudget, error) {
	for _, l := range m.listers {
		pdbs, err := l.GetPodPodDisruptionBudgets(pod)
		if err == nil {
			return pdbs, nil
		}
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{Group: "policy", Resource: "poddisruptionbudgets"}, pod.Name)
}

// ---------------------------------------------------------------------------
// ReplicaSet
// ---------------------------------------------------------------------------

type multiReplicaSetLister struct {
	listers []appsv1lister.ReplicaSetLister
}

func (m *multiReplicaSetLister) List(selector labels.Selector) ([]*appsv1.ReplicaSet, error) {
	return listAll(selector, m.listers)
}

func (m *multiReplicaSetLister) ReplicaSets(namespace string) appsv1lister.ReplicaSetNamespaceLister {
	return &multiNamespace[*appsv1.ReplicaSet, appsv1lister.ReplicaSetNamespaceLister]{
		listers: nsAll(m.listers, namespace, appsv1lister.ReplicaSetLister.ReplicaSets),
		gr:      schema.GroupResource{Group: "apps", Resource: "replicasets"},
	}
}

func (m *multiReplicaSetLister) GetPodReplicaSets(pod *corev1.Pod) ([]*appsv1.ReplicaSet, error) {
	for _, l := range m.listers {
		rss, err := l.GetPodReplicaSets(pod)
		if err == nil {
			return rss, nil
		}
	}
	return nil, fmt.Errorf("no replicasets found for pod %s/%s", pod.Namespace, pod.Name)
}

// ---------------------------------------------------------------------------
// StatefulSet
// ---------------------------------------------------------------------------

type multiStatefulSetLister struct {
	listers []appsv1lister.StatefulSetLister
}

func (m *multiStatefulSetLister) List(selector labels.Selector) ([]*appsv1.StatefulSet, error) {
	return listAll(selector, m.listers)
}

func (m *multiStatefulSetLister) StatefulSets(namespace string) appsv1lister.StatefulSetNamespaceLister {
	return &multiNamespace[*appsv1.StatefulSet, appsv1lister.StatefulSetNamespaceLister]{
		listers: nsAll(m.listers, namespace, appsv1lister.StatefulSetLister.StatefulSets),
		gr:      schema.GroupResource{Group: "apps", Resource: "statefulsets"},
	}
}

func (m *multiStatefulSetLister) GetPodStatefulSets(pod *corev1.Pod) ([]*appsv1.StatefulSet, error) {
	for _, l := range m.listers {
		sl, ok := interface{}(l).(interface {
			GetPodStatefulSets(*corev1.Pod) ([]*appsv1.StatefulSet, error)
		})
		if ok {
			ss, err := sl.GetPodStatefulSets(pod)
			if err == nil {
				return ss, nil
			}
		}
	}
	return nil, fmt.Errorf("no statefulsets found for pod %s/%s", pod.Namespace, pod.Name)
}
