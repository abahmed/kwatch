package controller

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	appsv1lister "k8s.io/client-go/listers/apps/v1"
	batchv1lister "k8s.io/client-go/listers/batch/v1"
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
	return nil, fmt.Errorf("pod %q not found in any namespace lister", name)
}

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
	return nil, fmt.Errorf("replicaset %q not found in any namespace lister", name)
}

type multiDeploymentLister struct {
	listers []appsv1lister.DeploymentLister
}

func (m *multiDeploymentLister) List(selector labels.Selector) ([]*appsv1.Deployment, error) {
	var all []*appsv1.Deployment
	for _, l := range m.listers {
		items, err := l.List(selector)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}

func (m *multiDeploymentLister) Deployments(namespace string) appsv1lister.DeploymentNamespaceLister {
	nsl := make([]appsv1lister.DeploymentNamespaceLister, 0, len(m.listers))
	for _, l := range m.listers {
		nsl = append(nsl, l.Deployments(namespace))
	}
	return &multiDeploymentNamespaceLister{listers: nsl}
}

type multiDeploymentNamespaceLister struct {
	listers []appsv1lister.DeploymentNamespaceLister
}

func (m *multiDeploymentNamespaceLister) List(selector labels.Selector) ([]*appsv1.Deployment, error) {
	var all []*appsv1.Deployment
	for _, l := range m.listers {
		items, err := l.List(selector)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}

func (m *multiDeploymentNamespaceLister) Get(name string) (*appsv1.Deployment, error) {
	for _, l := range m.listers {
		item, err := l.Get(name)
		if err == nil {
			return item, nil
		}
	}
	return nil, fmt.Errorf("deployment %q not found in any namespace lister", name)
}

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
	return nil, fmt.Errorf("job %q not found in any namespace lister", name)
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
