package controller

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	appsv1lister "k8s.io/client-go/listers/apps/v1"
)

type multiStatefulSetLister struct {
	listers []appsv1lister.StatefulSetLister
}

func (m *multiStatefulSetLister) List(selector labels.Selector) ([]*appsv1.StatefulSet, error) {
	var all []*appsv1.StatefulSet
	for _, l := range m.listers {
		items, err := l.List(selector)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}

func (m *multiStatefulSetLister) StatefulSets(namespace string) appsv1lister.StatefulSetNamespaceLister {
	nsl := make([]appsv1lister.StatefulSetNamespaceLister, 0, len(m.listers))
	for _, l := range m.listers {
		nsl = append(nsl, l.StatefulSets(namespace))
	}
	return &multiStatefulSetNamespaceLister{listers: nsl}
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

type multiStatefulSetNamespaceLister struct {
	listers []appsv1lister.StatefulSetNamespaceLister
}

func (m *multiStatefulSetNamespaceLister) List(selector labels.Selector) ([]*appsv1.StatefulSet, error) {
	var all []*appsv1.StatefulSet
	for _, l := range m.listers {
		items, err := l.List(selector)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}

func (m *multiStatefulSetNamespaceLister) Get(name string) (*appsv1.StatefulSet, error) {
	for _, l := range m.listers {
		item, err := l.Get(name)
		if err == nil {
			return item, nil
		}
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{Group: "apps", Resource: "statefulsets"}, name)
}
