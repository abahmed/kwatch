package controller

import (
	admissionregistrationv1lister "k8s.io/client-go/listers/admissionregistration/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
	networkingv1lister "k8s.io/client-go/listers/networking/v1"
	storagev1lister "k8s.io/client-go/listers/storage/v1"
	"k8s.io/client-go/tools/cache"
)

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

func (fs factorySet) pvcInformers() []cache.SharedIndexInformer {
	if fs.global != nil {
		return []cache.SharedIndexInformer{fs.global.Core().V1().PersistentVolumeClaims().Informer()}
	}
	out := make([]cache.SharedIndexInformer, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		out = append(out, f.Core().V1().PersistentVolumeClaims().Informer())
	}
	return out
}

func (fs factorySet) pvcLister() corev1lister.PersistentVolumeClaimLister {
	if fs.global != nil {
		return fs.global.Core().V1().PersistentVolumeClaims().Lister()
	}
	listers := make([]corev1lister.PersistentVolumeClaimLister, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		listers = append(listers, f.Core().V1().PersistentVolumeClaims().Lister())
	}
	return &multiPVCListLister{listers: listers}
}

func (fs factorySet) serviceAccountLister() corev1lister.ServiceAccountLister {
	if fs.global != nil {
		return fs.global.Core().V1().ServiceAccounts().Lister()
	}
	listers := make([]corev1lister.ServiceAccountLister, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		listers = append(listers, f.Core().V1().ServiceAccounts().Lister())
	}
	return &multiServiceAccountLister{listers: listers}
}

func (fs factorySet) serviceAccountInformers() []cache.SharedIndexInformer {
	if fs.global != nil {
		return []cache.SharedIndexInformer{fs.global.Core().V1().ServiceAccounts().Informer()}
	}
	out := make([]cache.SharedIndexInformer, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		out = append(out, f.Core().V1().ServiceAccounts().Informer())
	}
	return out
}

func (fs factorySet) persistentVolumeLister() corev1lister.PersistentVolumeLister {
	if fs.clusterScoped != nil {
		return fs.clusterScoped.Core().V1().PersistentVolumes().Lister()
	}
	if fs.global != nil {
		return fs.global.Core().V1().PersistentVolumes().Lister()
	}
	return nil
}

func (fs factorySet) persistentVolumeInformers() []cache.SharedIndexInformer {
	if fs.clusterScoped != nil {
		return []cache.SharedIndexInformer{fs.clusterScoped.Core().V1().PersistentVolumes().Informer()}
	}
	if fs.global != nil {
		return []cache.SharedIndexInformer{fs.global.Core().V1().PersistentVolumes().Informer()}
	}
	return nil
}

func (fs factorySet) storageClassLister() storagev1lister.StorageClassLister {
	if fs.clusterScoped != nil {
		return fs.clusterScoped.Storage().V1().StorageClasses().Lister()
	}
	if fs.global != nil {
		return fs.global.Storage().V1().StorageClasses().Lister()
	}
	return nil
}

func (fs factorySet) storageClassInformers() []cache.SharedIndexInformer {
	if fs.clusterScoped != nil {
		return []cache.SharedIndexInformer{fs.clusterScoped.Storage().V1().StorageClasses().Informer()}
	}
	if fs.global != nil {
		return []cache.SharedIndexInformer{fs.global.Storage().V1().StorageClasses().Informer()}
	}
	return nil
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
