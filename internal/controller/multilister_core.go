package controller

import (
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	coordinationv1lister "k8s.io/client-go/listers/coordination/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
)

type multiLeaseLister struct {
	listers []coordinationv1lister.LeaseLister
}

func (m *multiLeaseLister) List(selector labels.Selector) ([]*coordinationv1.Lease, error) {
	return listAll(selector, m.listers)
}

func (m *multiLeaseLister) Leases(namespace string) coordinationv1lister.LeaseNamespaceLister {
	return &multiNamespace[*coordinationv1.Lease, coordinationv1lister.LeaseNamespaceLister]{
		listers: nsAll(m.listers, namespace, coordinationv1lister.LeaseLister.Leases),
		gr:      schema.GroupResource{Group: "coordination.k8s.io", Resource: "leases"},
	}
}

// ConfigMap
// ---------------------------------------------------------------------------

type multiConfigMapLister struct {
	listers []corev1lister.ConfigMapLister
}

func (m *multiConfigMapLister) List(selector labels.Selector) ([]*corev1.ConfigMap, error) {
	return listAll(selector, m.listers)
}

func (m *multiConfigMapLister) ConfigMaps(namespace string) corev1lister.ConfigMapNamespaceLister {
	return &multiNamespace[*corev1.ConfigMap, corev1lister.ConfigMapNamespaceLister]{
		listers: nsAll(m.listers, namespace, corev1lister.ConfigMapLister.ConfigMaps),
		gr:      schema.GroupResource{Group: "", Resource: "configmaps"},
	}
}

// ---------------------------------------------------------------------------
// Event
// ---------------------------------------------------------------------------

type multiEventLister struct {
	listers []corev1lister.EventLister
}

func (m *multiEventLister) List(selector labels.Selector) ([]*corev1.Event, error) {
	return listAll(selector, m.listers)
}

func (m *multiEventLister) Events(namespace string) corev1lister.EventNamespaceLister {
	return &multiNamespace[*corev1.Event, corev1lister.EventNamespaceLister]{
		listers: nsAll(m.listers, namespace, corev1lister.EventLister.Events),
		gr:      schema.GroupResource{Group: "", Resource: "events"},
	}
}

// ---------------------------------------------------------------------------
// Pod
// ---------------------------------------------------------------------------

type multiPodLister struct {
	listers []corev1lister.PodLister
}

func (m *multiPodLister) List(selector labels.Selector) ([]*corev1.Pod, error) {
	return listAll(selector, m.listers)
}

func (m *multiPodLister) Pods(namespace string) corev1lister.PodNamespaceLister {
	return &multiNamespace[*corev1.Pod, corev1lister.PodNamespaceLister]{
		listers: nsAll(m.listers, namespace, corev1lister.PodLister.Pods),
		gr:      schema.GroupResource{Group: "", Resource: "pods"},
	}
}

// ---------------------------------------------------------------------------
// PersistentVolumeClaim
// ---------------------------------------------------------------------------

type multiPVCListLister struct {
	listers []corev1lister.PersistentVolumeClaimLister
}

// ResourceQuota
// ---------------------------------------------------------------------------

type multiResourceQuotaLister struct {
	listers []corev1lister.ResourceQuotaLister
}

func (m *multiResourceQuotaLister) List(selector labels.Selector) ([]*corev1.ResourceQuota, error) {
	return listAll(selector, m.listers)
}

func (m *multiResourceQuotaLister) ResourceQuotas(namespace string) corev1lister.ResourceQuotaNamespaceLister {
	return &multiNamespace[*corev1.ResourceQuota, corev1lister.ResourceQuotaNamespaceLister]{
		listers: nsAll(m.listers, namespace, corev1lister.ResourceQuotaLister.ResourceQuotas),
		gr:      schema.GroupResource{Group: "", Resource: "resourcequotas"},
	}
}

func (m *multiPVCListLister) List(selector labels.Selector) ([]*corev1.PersistentVolumeClaim, error) {
	return listAll(selector, m.listers)
}

func (m *multiPVCListLister) PersistentVolumeClaims(namespace string) corev1lister.PersistentVolumeClaimNamespaceLister {
	return &multiNamespace[*corev1.PersistentVolumeClaim, corev1lister.PersistentVolumeClaimNamespaceLister]{
		listers: nsAll(m.listers, namespace, corev1lister.PersistentVolumeClaimLister.PersistentVolumeClaims),
		gr:      schema.GroupResource{Group: "", Resource: "persistentvolumeclaims"},
	}
}

// ---------------------------------------------------------------------------
// Secret
// ---------------------------------------------------------------------------

type multiSecretLister struct {
	listers []corev1lister.SecretLister
}

func (m *multiSecretLister) List(selector labels.Selector) ([]*corev1.Secret, error) {
	return listAll(selector, m.listers)
}

func (m *multiSecretLister) Secrets(namespace string) corev1lister.SecretNamespaceLister {
	return &multiNamespace[*corev1.Secret, corev1lister.SecretNamespaceLister]{
		listers: nsAll(m.listers, namespace, corev1lister.SecretLister.Secrets),
		gr:      schema.GroupResource{Group: "", Resource: "secrets"},
	}
}

// ---------------------------------------------------------------------------
// Service
// ---------------------------------------------------------------------------

type multiServiceLister struct {
	listers []corev1lister.ServiceLister
}

func (m *multiServiceLister) List(selector labels.Selector) ([]*corev1.Service, error) {
	return listAll(selector, m.listers)
}

func (m *multiServiceLister) Services(namespace string) corev1lister.ServiceNamespaceLister {
	return &multiNamespace[*corev1.Service, corev1lister.ServiceNamespaceLister]{
		listers: nsAll(m.listers, namespace, corev1lister.ServiceLister.Services),
		gr:      schema.GroupResource{Group: "", Resource: "services"},
	}
}

// ---------------------------------------------------------------------------
// ServiceAccount
// ---------------------------------------------------------------------------

type multiServiceAccountLister struct {
	listers []corev1lister.ServiceAccountLister
}

func (m *multiServiceAccountLister) List(selector labels.Selector) ([]*corev1.ServiceAccount, error) {
	return listAll(selector, m.listers)
}

func (m *multiServiceAccountLister) ServiceAccounts(namespace string) corev1lister.ServiceAccountNamespaceLister {
	return &multiNamespace[*corev1.ServiceAccount, corev1lister.ServiceAccountNamespaceLister]{
		listers: nsAll(m.listers, namespace, corev1lister.ServiceAccountLister.ServiceAccounts),
		gr:      schema.GroupResource{Group: "", Resource: "serviceaccounts"},
	}
}

// ---------------------------------------------------------------------------
