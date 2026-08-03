package controller

import (
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	autoscalingv2lister "k8s.io/client-go/listers/autoscaling/v2"
)

type multiHorizontalPodAutoscalerLister struct {
	listers []autoscalingv2lister.HorizontalPodAutoscalerLister
}

func (m *multiHorizontalPodAutoscalerLister) List(selector labels.Selector) ([]*autoscalingv2.HorizontalPodAutoscaler, error) {
	var all []*autoscalingv2.HorizontalPodAutoscaler
	for _, l := range m.listers {
		items, err := l.List(selector)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}

func (m *multiHorizontalPodAutoscalerLister) HorizontalPodAutoscalers(namespace string) autoscalingv2lister.HorizontalPodAutoscalerNamespaceLister {
	nsl := make([]autoscalingv2lister.HorizontalPodAutoscalerNamespaceLister, 0, len(m.listers))
	for _, l := range m.listers {
		nsl = append(nsl, l.HorizontalPodAutoscalers(namespace))
	}
	return &multiHorizontalPodAutoscalerNamespaceLister{listers: nsl}
}

type multiHorizontalPodAutoscalerNamespaceLister struct {
	listers []autoscalingv2lister.HorizontalPodAutoscalerNamespaceLister
}

func (m *multiHorizontalPodAutoscalerNamespaceLister) List(selector labels.Selector) ([]*autoscalingv2.HorizontalPodAutoscaler, error) {
	var all []*autoscalingv2.HorizontalPodAutoscaler
	for _, l := range m.listers {
		items, err := l.List(selector)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}

func (m *multiHorizontalPodAutoscalerNamespaceLister) Get(name string) (*autoscalingv2.HorizontalPodAutoscaler, error) {
	for _, l := range m.listers {
		item, err := l.Get(name)
		if err == nil {
			return item, nil
		}
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{Group: "autoscaling", Resource: "horizontalpodautoscalers"}, name)
}
