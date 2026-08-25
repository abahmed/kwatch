package handler

import (
	admissionregistrationv1lister "k8s.io/client-go/listers/admissionregistration/v1"
	appsv1lister "k8s.io/client-go/listers/apps/v1"
	autoscalingv2lister "k8s.io/client-go/listers/autoscaling/v2"
	batchv1lister "k8s.io/client-go/listers/batch/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
	discoveryv1lister "k8s.io/client-go/listers/discovery/v1"
	networkingv1lister "k8s.io/client-go/listers/networking/v1"
	policyv1lister "k8s.io/client-go/listers/policy/v1"

	"github.com/abahmed/kwatch/internal/insight"
)

// listerSet bundles every informer-backed lister the handler reads from.
// The controller wires them individually as each informer starts.
type listerSet struct {
	pod           corev1lister.PodLister
	node          corev1lister.NodeLister
	deploy        appsv1lister.DeploymentLister
	job           batchv1lister.JobLister
	cronJob       batchv1lister.CronJobLister
	rs            appsv1lister.ReplicaSetLister
	ds            appsv1lister.DaemonSetLister
	ss            appsv1lister.StatefulSetLister
	pdb           policyv1lister.PodDisruptionBudgetLister
	event         corev1lister.EventLister
	hpa           autoscalingv2lister.HorizontalPodAutoscalerLister
	mwc           admissionregistrationv1lister.MutatingWebhookConfigurationLister
	vwc           admissionregistrationv1lister.ValidatingWebhookConfigurationLister
	service       corev1lister.ServiceLister
	endpointSlice discoveryv1lister.EndpointSliceLister
	secret        corev1lister.SecretLister
	netpol        networkingv1lister.NetworkPolicyLister
	ingress       networkingv1lister.IngressLister
	cpPod         corev1lister.PodLister
}

func (h *handler) SetPodLister(lister corev1lister.PodLister) {
	h.listers.pod = lister
}

func (h *handler) SetNodeLister(lister corev1lister.NodeLister) {
	h.listers.node = lister
}

func (h *handler) SetDeploymentLister(lister appsv1lister.DeploymentLister) {
	h.listers.deploy = lister
	h.correlator.SetDeployLister(lister)
}

func (h *handler) SetJobLister(lister batchv1lister.JobLister) {
	h.listers.job = lister
}

func (h *handler) SetReplicaLister(lister appsv1lister.ReplicaSetLister) {
	h.listers.rs = lister
}

func (h *handler) SetDaemonSetLister(lister appsv1lister.DaemonSetLister) {
	h.listers.ds = lister
	h.correlator.SetDaemonSetLister(lister)
}

func (h *handler) SetStatefulSetLister(lister appsv1lister.StatefulSetLister) {
	h.listers.ss = lister
	h.correlator.SetStatefulSetLister(lister)
}

func (h *handler) SetEventLister(lister corev1lister.EventLister) {
	h.listers.event = lister
}

func (h *handler) SetPdbLister(lister policyv1lister.PodDisruptionBudgetLister) {
	h.listers.pdb = lister
}

func (h *handler) SetHorizontalPodAutoscalerLister(lister autoscalingv2lister.HorizontalPodAutoscalerLister) {
	h.listers.hpa = lister
}

func (h *handler) SetMwCLister(lister admissionregistrationv1lister.MutatingWebhookConfigurationLister) {
	h.listers.mwc = lister
}

func (h *handler) SetVwCLister(lister admissionregistrationv1lister.ValidatingWebhookConfigurationLister) {
	h.listers.vwc = lister
}

func (h *handler) SetServiceLister(lister corev1lister.ServiceLister) {
	h.listers.service = lister
	h.correlator.SetServiceLister(lister)
}

func (h *handler) SetEndpointSliceLister(lister discoveryv1lister.EndpointSliceLister) {
	h.listers.endpointSlice = lister
}

func (h *handler) SetSecretLister(lister corev1lister.SecretLister) {
	h.listers.secret = lister
}

func (h *handler) SetInsightEngine(engine *insight.Engine) {
	h.insightEngine = engine
}

func (h *handler) SetIngressLister(lister networkingv1lister.IngressLister) {
	h.listers.ingress = lister
}

func (h *handler) SetNetpolLister(lister networkingv1lister.NetworkPolicyLister) {
	h.listers.netpol = lister
}

func (h *handler) SetCpPodLister(lister corev1lister.PodLister) {
	h.listers.cpPod = lister
}

func (h *handler) SetCronJobLister(lister batchv1lister.CronJobLister) {
	h.listers.cronJob = lister
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
