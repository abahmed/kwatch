package controller

import (
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	corev1lister "k8s.io/client-go/listers/core/v1"
)

type multiPVCListLister struct {
	listers []corev1lister.PersistentVolumeClaimLister
}

func (m *multiPVCListLister) List(selector labels.Selector) ([]*corev1.PersistentVolumeClaim, error) {
	var all []*corev1.PersistentVolumeClaim
	for _, l := range m.listers {
		items, err := l.List(selector)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}

func (m *multiPVCListLister) PersistentVolumeClaims(namespace string) corev1lister.PersistentVolumeClaimNamespaceLister {
	nsl := make([]corev1lister.PersistentVolumeClaimNamespaceLister, 0, len(m.listers))
	for _, l := range m.listers {
		nsl = append(nsl, l.PersistentVolumeClaims(namespace))
	}
	return &multiPvcNamespaceLister{listers: nsl}
}

type multiPvcNamespaceLister struct {
	listers []corev1lister.PersistentVolumeClaimNamespaceLister
}

func (m *multiPvcNamespaceLister) List(selector labels.Selector) ([]*corev1.PersistentVolumeClaim, error) {
	var all []*corev1.PersistentVolumeClaim
	for _, l := range m.listers {
		items, err := l.List(selector)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}

func (m *multiPvcNamespaceLister) Get(name string) (*corev1.PersistentVolumeClaim, error) {
	for _, l := range m.listers {
		item, err := l.Get(name)
		if err == nil {
			return item, nil
		}
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{Group: "", Resource: "persistentvolumeclaims"}, name)
}
