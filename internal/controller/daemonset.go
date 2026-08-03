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

type multiDaemonSetLister struct {
	listers []appsv1lister.DaemonSetLister
}

func (m *multiDaemonSetLister) List(selector labels.Selector) ([]*appsv1.DaemonSet, error) {
	var all []*appsv1.DaemonSet
	for _, l := range m.listers {
		items, err := l.List(selector)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}

func (m *multiDaemonSetLister) DaemonSets(namespace string) appsv1lister.DaemonSetNamespaceLister {
	nsl := make([]appsv1lister.DaemonSetNamespaceLister, 0, len(m.listers))
	for _, l := range m.listers {
		nsl = append(nsl, l.DaemonSets(namespace))
	}
	return &multiDaemonSetNamespaceLister{listers: nsl}
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

type multiDaemonSetNamespaceLister struct {
	listers []appsv1lister.DaemonSetNamespaceLister
}

func (m *multiDaemonSetNamespaceLister) List(selector labels.Selector) ([]*appsv1.DaemonSet, error) {
	var all []*appsv1.DaemonSet
	for _, l := range m.listers {
		items, err := l.List(selector)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}

func (m *multiDaemonSetNamespaceLister) Get(name string) (*appsv1.DaemonSet, error) {
	for _, l := range m.listers {
		item, err := l.Get(name)
		if err == nil {
			return item, nil
		}
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{Group: "apps", Resource: "daemonsets"}, name)
}
