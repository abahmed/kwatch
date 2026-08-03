package controller

import (
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	batchv1lister "k8s.io/client-go/listers/batch/v1"
)

type multiJobLister struct {
	listers []batchv1lister.JobLister
}

func (m *multiJobLister) List(selector labels.Selector) ([]*batchv1.Job, error) {
	var all []*batchv1.Job
	for _, l := range m.listers {
		items, err := l.List(selector)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}

func (m *multiJobLister) Jobs(namespace string) batchv1lister.JobNamespaceLister {
	nsl := make([]batchv1lister.JobNamespaceLister, 0, len(m.listers))
	for _, l := range m.listers {
		nsl = append(nsl, l.Jobs(namespace))
	}
	return &multiJobNamespaceLister{listers: nsl}
}

type multiJobNamespaceLister struct {
	listers []batchv1lister.JobNamespaceLister
}

func (m *multiJobNamespaceLister) List(selector labels.Selector) ([]*batchv1.Job, error) {
	var all []*batchv1.Job
	for _, l := range m.listers {
		items, err := l.List(selector)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}

func (m *multiJobNamespaceLister) Get(name string) (*batchv1.Job, error) {
	for _, l := range m.listers {
		item, err := l.Get(name)
		if err == nil {
			return item, nil
		}
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{Group: "batch", Resource: "jobs"}, name)
}

func (m *multiJobLister) GetPodJobs(pod *corev1.Pod) ([]batchv1.Job, error) {
	for _, l := range m.listers {
		jobs, err := l.GetPodJobs(pod)
		if err == nil {
			return jobs, nil
		}
	}
	return nil, fmt.Errorf("no jobs found for pod %s/%s", pod.Namespace, pod.Name)
}
