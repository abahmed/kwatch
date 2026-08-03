package controller

import (
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	corev1lister "k8s.io/client-go/listers/core/v1"
)

type multiPodLister struct {
	listers []corev1lister.PodLister
}

func (m *multiPodLister) List(selector labels.Selector) ([]*corev1.Pod, error) {
	var all []*corev1.Pod
	for _, l := range m.listers {
		pods, err := l.List(selector)
		if err != nil {
			return nil, err
		}
		all = append(all, pods...)
	}
	return all, nil
}

func (m *multiPodLister) Pods(namespace string) corev1lister.PodNamespaceLister {
	nsl := make([]corev1lister.PodNamespaceLister, 0, len(m.listers))
	for _, l := range m.listers {
		nsl = append(nsl, l.Pods(namespace))
	}
	return &multiPodNamespaceLister{listers: nsl}
}

type multiPodNamespaceLister struct {
	listers []corev1lister.PodNamespaceLister
}

func (m *multiPodNamespaceLister) List(selector labels.Selector) ([]*corev1.Pod, error) {
	var all []*corev1.Pod
	for _, l := range m.listers {
		pods, err := l.List(selector)
		if err != nil {
			return nil, err
		}
		all = append(all, pods...)
	}
	return all, nil
}

func (m *multiPodNamespaceLister) Get(name string) (*corev1.Pod, error) {
	for _, l := range m.listers {
		pod, err := l.Get(name)
		if err == nil {
			return pod, nil
		}
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, name)
}
