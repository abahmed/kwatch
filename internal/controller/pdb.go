package controller

import (
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	policyv1lister "k8s.io/client-go/listers/policy/v1"
)

type multiPodDisruptionBudgetLister struct {
	listers []policyv1lister.PodDisruptionBudgetLister
}

func (m *multiPodDisruptionBudgetLister) List(selector labels.Selector) ([]*policyv1.PodDisruptionBudget, error) {
	var all []*policyv1.PodDisruptionBudget
	for _, l := range m.listers {
		items, err := l.List(selector)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}

func (m *multiPodDisruptionBudgetLister) GetPodPodDisruptionBudgets(pod *corev1.Pod) ([]*policyv1.PodDisruptionBudget, error) {
	for _, l := range m.listers {
		pdbs, err := l.GetPodPodDisruptionBudgets(pod)
		if err == nil {
			return pdbs, nil
		}
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{Group: "policy", Resource: "poddisruptionbudgets"}, pod.Name)
}

func (m *multiPodDisruptionBudgetLister) PodDisruptionBudgets(namespace string) policyv1lister.PodDisruptionBudgetNamespaceLister {
	nsl := make([]policyv1lister.PodDisruptionBudgetNamespaceLister, 0, len(m.listers))
	for _, l := range m.listers {
		nsl = append(nsl, l.PodDisruptionBudgets(namespace))
	}
	return &multiPodDisruptionBudgetNamespaceLister{listers: nsl}
}

type multiPodDisruptionBudgetNamespaceLister struct {
	listers []policyv1lister.PodDisruptionBudgetNamespaceLister
}

func (m *multiPodDisruptionBudgetNamespaceLister) List(selector labels.Selector) ([]*policyv1.PodDisruptionBudget, error) {
	var all []*policyv1.PodDisruptionBudget
	for _, l := range m.listers {
		items, err := l.List(selector)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}

func (m *multiPodDisruptionBudgetNamespaceLister) Get(name string) (*policyv1.PodDisruptionBudget, error) {
	for _, l := range m.listers {
		item, err := l.Get(name)
		if err == nil {
			return item, nil
		}
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{Group: "policy", Resource: "poddisruptionbudgets"}, name)
}
