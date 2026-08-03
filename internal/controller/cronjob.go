package controller

import (
	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	batchv1lister "k8s.io/client-go/listers/batch/v1"
)

type multiCronJobLister struct {
	listers []batchv1lister.CronJobLister
}

func (m *multiCronJobLister) List(selector labels.Selector) ([]*batchv1.CronJob, error) {
	var all []*batchv1.CronJob
	for _, l := range m.listers {
		items, err := l.List(selector)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}

func (m *multiCronJobLister) CronJobs(namespace string) batchv1lister.CronJobNamespaceLister {
	nsl := make([]batchv1lister.CronJobNamespaceLister, 0, len(m.listers))
	for _, l := range m.listers {
		nsl = append(nsl, l.CronJobs(namespace))
	}
	return &multiCronJobNamespaceLister{listers: nsl}
}

type multiCronJobNamespaceLister struct {
	listers []batchv1lister.CronJobNamespaceLister
}

func (m *multiCronJobNamespaceLister) List(selector labels.Selector) ([]*batchv1.CronJob, error) {
	var all []*batchv1.CronJob
	for _, l := range m.listers {
		items, err := l.List(selector)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}

func (m *multiCronJobNamespaceLister) Get(name string) (*batchv1.CronJob, error) {
	for _, l := range m.listers {
		item, err := l.Get(name)
		if err == nil {
			return item, nil
		}
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{Group: "batch", Resource: "cronjobs"}, name)
}
