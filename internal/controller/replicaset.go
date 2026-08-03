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

type multiReplicaSetLister struct {
	listers []appsv1lister.ReplicaSetLister
}

func (m *multiReplicaSetLister) List(selector labels.Selector) ([]*appsv1.ReplicaSet, error) {
	var all []*appsv1.ReplicaSet
	for _, l := range m.listers {
		items, err := l.List(selector)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}

func (m *multiReplicaSetLister) ReplicaSets(namespace string) appsv1lister.ReplicaSetNamespaceLister {
	nsl := make([]appsv1lister.ReplicaSetNamespaceLister, 0, len(m.listers))
	for _, l := range m.listers {
		nsl = append(nsl, l.ReplicaSets(namespace))
	}
	return &multiReplicaSetNamespaceLister{listers: nsl}
}

type multiReplicaSetNamespaceLister struct {
	listers []appsv1lister.ReplicaSetNamespaceLister
}

func (m *multiReplicaSetNamespaceLister) List(selector labels.Selector) ([]*appsv1.ReplicaSet, error) {
	var all []*appsv1.ReplicaSet
	for _, l := range m.listers {
		items, err := l.List(selector)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
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

func (m *multiReplicaSetNamespaceLister) Get(name string) (*appsv1.ReplicaSet, error) {
	for _, l := range m.listers {
		item, err := l.Get(name)
		if err == nil {
			return item, nil
		}
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{Group: "apps", Resource: "replicasets"}, name)
}
