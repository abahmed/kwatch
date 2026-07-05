package handler

import (
	"fmt"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	admissionregistrationv1lister "k8s.io/client-go/listers/admissionregistration/v1"
	appsv1lister "k8s.io/client-go/listers/apps/v1"
	autoscalingv2lister "k8s.io/client-go/listers/autoscaling/v2"
	batchv1lister "k8s.io/client-go/listers/batch/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
	networkingv1lister "k8s.io/client-go/listers/networking/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// errorPodLister wraps a real PodLister but returns errors from Get
type errorPodLister struct{ corev1lister.PodLister }

func (e *errorPodLister) Pods(ns string) corev1lister.PodNamespaceLister {
	return &errorPodNSLister{PodNamespaceLister: e.PodLister.Pods(ns)}
}

type errorPodNSLister struct{ corev1lister.PodNamespaceLister }

func (e *errorPodNSLister) Get(name string) (*corev1.Pod, error) {
	return nil, fmt.Errorf("mock pod lister error")
}

// errorNodeLister wraps a real NodeLister but returns errors from Get
type errorNodeLister struct{ corev1lister.NodeLister }

func (e *errorNodeLister) Get(name string) (*corev1.Node, error) {
	return nil, fmt.Errorf("mock node lister error")
}

// errorDeployLister wraps a real DeploymentLister but returns errors from Get
type errorDeployLister struct{ appsv1lister.DeploymentLister }

func (e *errorDeployLister) Deployments(ns string) appsv1lister.DeploymentNamespaceLister {
	return &errorDeployNSLister{DeploymentNamespaceLister: e.DeploymentLister.Deployments(ns)}
}

type errorDeployNSLister struct{ appsv1lister.DeploymentNamespaceLister }

func (e *errorDeployNSLister) Get(name string) (*appsv1.Deployment, error) {
	return nil, fmt.Errorf("mock deploy lister error")
}

// errorDSLister wraps a real DaemonSetLister but returns errors from Get
type errorDSLister struct{ appsv1lister.DaemonSetLister }

func (e *errorDSLister) DaemonSets(ns string) appsv1lister.DaemonSetNamespaceLister {
	return &errorDSNSLister{DaemonSetNamespaceLister: e.DaemonSetLister.DaemonSets(ns)}
}

type errorDSNSLister struct{ appsv1lister.DaemonSetNamespaceLister }

func (e *errorDSNSLister) Get(name string) (*appsv1.DaemonSet, error) {
	return nil, fmt.Errorf("mock ds lister error")
}

// errorSSLister wraps a real StatefulSetLister but returns errors from Get
type errorSSLister struct{ appsv1lister.StatefulSetLister }

func (e *errorSSLister) StatefulSets(ns string) appsv1lister.StatefulSetNamespaceLister {
	return &errorSSNSLister{StatefulSetNamespaceLister: e.StatefulSetLister.StatefulSets(ns)}
}

type errorSSNSLister struct{ appsv1lister.StatefulSetNamespaceLister }

func (e *errorSSNSLister) Get(name string) (*appsv1.StatefulSet, error) {
	return nil, fmt.Errorf("mock ss lister error")
}

// errorJobLister wraps a real JobLister but returns errors from Get
type errorJobLister struct{ batchv1lister.JobLister }

func (e *errorJobLister) Jobs(ns string) batchv1lister.JobNamespaceLister {
	return &errorJobNSLister{JobNamespaceLister: e.JobLister.Jobs(ns)}
}

type errorJobNSLister struct{ batchv1lister.JobNamespaceLister }

func (e *errorJobNSLister) Get(name string) (*batchv1.Job, error) {
	return nil, fmt.Errorf("mock job lister error")
}

// errorCJLister wraps a real CronJobLister but returns errors from Get
type errorCJLister struct{ batchv1lister.CronJobLister }

func (e *errorCJLister) CronJobs(ns string) batchv1lister.CronJobNamespaceLister {
	return &errorCJNSLister{CronJobNamespaceLister: e.CronJobLister.CronJobs(ns)}
}

type errorCJNSLister struct{ batchv1lister.CronJobNamespaceLister }

func (e *errorCJNSLister) Get(name string) (*batchv1.CronJob, error) {
	return nil, fmt.Errorf("mock cronjob lister error")
}

// errorHPALister wraps a real HPA lister but returns errors from Get
type errorHPALister struct{ autoscalingv2lister.HorizontalPodAutoscalerLister }

func (e *errorHPALister) HorizontalPodAutoscalers(ns string) autoscalingv2lister.HorizontalPodAutoscalerNamespaceLister {
	return &errorHPANSLister{HorizontalPodAutoscalerNamespaceLister: e.HorizontalPodAutoscalerLister.HorizontalPodAutoscalers(ns)}
}

type errorHPANSLister struct{ autoscalingv2lister.HorizontalPodAutoscalerNamespaceLister }

func (e *errorHPANSLister) Get(name string) (*autoscalingv2.HorizontalPodAutoscaler, error) {
	return nil, fmt.Errorf("mock hpa lister error")
}

// errorServiceLister wraps a real ServiceLister but returns errors from Get
type errorServiceLister struct{ corev1lister.ServiceLister }

func (e *errorServiceLister) Services(ns string) corev1lister.ServiceNamespaceLister {
	return &errorServiceNSLister{ServiceNamespaceLister: e.ServiceLister.Services(ns)}
}

type errorServiceNSLister struct{ corev1lister.ServiceNamespaceLister }

func (e *errorServiceNSLister) Get(name string) (*corev1.Service, error) {
	return nil, fmt.Errorf("mock service lister error")
}

// errorEndpointLister wraps a real EndpointsLister but returns errors from Get
type errorEndpointLister struct{ corev1lister.EndpointsLister }

func (e *errorEndpointLister) Endpoints(ns string) corev1lister.EndpointsNamespaceLister {
	return &errorEndpointNSLister{EndpointsNamespaceLister: e.EndpointsLister.Endpoints(ns)}
}

type errorEndpointNSLister struct{ corev1lister.EndpointsNamespaceLister }

func (e *errorEndpointNSLister) Get(name string) (*corev1.Endpoints, error) {
	return nil, fmt.Errorf("mock endpoint lister error")
}

// errorNetpolLister wraps a real NetworkPolicyLister but returns errors from Get
type errorNetpolLister struct{ networkingv1lister.NetworkPolicyLister }

func (e *errorNetpolLister) NetworkPolicies(ns string) networkingv1lister.NetworkPolicyNamespaceLister {
	return &errorNetpolNSLister{NetworkPolicyNamespaceLister: e.NetworkPolicyLister.NetworkPolicies(ns)}
}

type errorNetpolNSLister struct{ networkingv1lister.NetworkPolicyNamespaceLister }

func (e *errorNetpolNSLister) Get(name string) (*networkingv1.NetworkPolicy, error) {
	return nil, fmt.Errorf("mock netpol lister error")
}

// errorIngressLister wraps a real IngressLister but returns errors from Get
type errorIngressLister struct{ networkingv1lister.IngressLister }

func (e *errorIngressLister) Ingresses(ns string) networkingv1lister.IngressNamespaceLister {
	return &errorIngressNSLister{IngressNamespaceLister: e.IngressLister.Ingresses(ns)}
}

type errorIngressNSLister struct{ networkingv1lister.IngressNamespaceLister }

func (e *errorIngressNSLister) Get(name string) (*networkingv1.Ingress, error) {
	return nil, fmt.Errorf("mock ingress lister error")
}

// errorMwCLister wraps a real MutatingWebhookConfigurationLister (cluster-scoped)
type errorMwCLister struct{ admissionregistrationv1lister.MutatingWebhookConfigurationLister }

func (e *errorMwCLister) Get(name string) (*admissionregistrationv1.MutatingWebhookConfiguration, error) {
	return nil, fmt.Errorf("mock mwc lister error")
}

// errorVwCLister wraps a real ValidatingWebhookConfigurationLister (cluster-scoped)
type errorVwCLister struct {
	admissionregistrationv1lister.ValidatingWebhookConfigurationLister
}

func (e *errorVwCLister) Get(name string) (*admissionregistrationv1.ValidatingWebhookConfiguration, error) {
	return nil, fmt.Errorf("mock vwc lister error")
}

// errorCpPodLister wraps a real PodLister but returns errors from List
type errorCpPodLister struct{ corev1lister.PodLister }

func (e *errorCpPodLister) List(selector labels.Selector) ([]*corev1.Pod, error) {
	return nil, fmt.Errorf("mock cp pod lister list error")
}

// errorSecretLister wraps a real SecretLister but returns errors from List
type errorSecretLister struct{ corev1lister.SecretLister }

func (e *errorSecretLister) List(selector labels.Selector) ([]*corev1.Secret, error) {
	return nil, fmt.Errorf("mock secret lister list error")
}

// errorEventLister wraps a real EventLister but returns errors from Events().List
type errorEventLister struct{ corev1lister.EventLister }

func (e *errorEventLister) Events(ns string) corev1lister.EventNamespaceLister {
	return &errorEventNSLister{EventNamespaceLister: e.EventLister.Events(ns)}
}

type errorEventNSLister struct{ corev1lister.EventNamespaceLister }

func (e *errorEventNSLister) List(selector labels.Selector) ([]*corev1.Event, error) {
	return nil, fmt.Errorf("mock event lister error")
}
