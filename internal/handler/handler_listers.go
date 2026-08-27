package handler

import (
	corev1 "k8s.io/api/core/v1"
	admregv1lister "k8s.io/client-go/listers/admissionregistration/v1"
	appsv1lister "k8s.io/client-go/listers/apps/v1"
	autoscalingv2lister "k8s.io/client-go/listers/autoscaling/v2"
	batchv1lister "k8s.io/client-go/listers/batch/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
	discoveryv1lister "k8s.io/client-go/listers/discovery/v1"
	networkingv1lister "k8s.io/client-go/listers/networking/v1"
	policyv1lister "k8s.io/client-go/listers/policy/v1"
)

// Listers bundles every informer-backed lister the handler reads from. The
// controller assembles it once, after wiring its informers, and hands it over
// with SetListers; a lister for a disabled monitor is simply nil. Replacing
// twenty individual setters with one value keeps the Handler interface about
// what the handler does, not how it is fed.
type Listers struct {
	Pod           corev1lister.PodLister
	Node          corev1lister.NodeLister
	Deploy        appsv1lister.DeploymentLister
	Job           batchv1lister.JobLister
	CronJob       batchv1lister.CronJobLister
	RS            appsv1lister.ReplicaSetLister
	DS            appsv1lister.DaemonSetLister
	SS            appsv1lister.StatefulSetLister
	PDB           policyv1lister.PodDisruptionBudgetLister
	Event         corev1lister.EventLister
	EventsByPod   func(namespace, pod string) ([]*corev1.Event, error)
	HPA           autoscalingv2lister.HorizontalPodAutoscalerLister
	MWC           admregv1lister.MutatingWebhookConfigurationLister
	VWC           admregv1lister.ValidatingWebhookConfigurationLister
	Service       corev1lister.ServiceLister
	EndpointSlice discoveryv1lister.EndpointSliceLister
	Secret        corev1lister.SecretLister
	Netpol        networkingv1lister.NetworkPolicyLister
	Ingress       networkingv1lister.IngressLister
	CPPod         corev1lister.PodLister
}

// SetListers installs the informer-backed lookups. The correlation engine gets
// the ones it needs for owner-health and topology checks at the same time, so
// the two can never disagree about which caches exist.
func (h *handler) SetListers(l Listers) {
	h.listers = l
	if l.Deploy != nil {
		h.correlator.SetDeployLister(l.Deploy)
	}
	if l.SS != nil {
		h.correlator.SetStatefulSetLister(l.SS)
	}
	if l.DS != nil {
		h.correlator.SetDaemonSetLister(l.DS)
	}
	if l.Service != nil {
		h.correlator.SetServiceLister(l.Service)
	}
}

func (h *handler) SetBaseline(baseline map[string]map[string]int64) {
	h.correlator.SetBaseline(baseline)
}

func (h *handler) SetActiveNodeIncidents(nodeNames []string) {
	h.correlator.SetActiveNodeIncidents(nodeNames)
}

func (h *handler) ClearBaselineForPod(namespace, podName string) {
	h.correlator.ClearBaselineForPod(namespace, podName)
}
