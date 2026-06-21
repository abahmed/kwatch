package controller

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	appsv1lister "k8s.io/client-go/listers/apps/v1"
	autoscalingv2lister "k8s.io/client-go/listers/autoscaling/v2"
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
	return nil, fmt.Errorf("daemonset %q not found in any namespace lister", name)
}

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
	return nil, fmt.Errorf("statefulset %q not found in any namespace lister", name)
}

type multiEventLister struct {
	listers []corev1lister.EventLister
}

func (m *multiEventLister) List(selector labels.Selector) ([]*corev1.Event, error) {
	var all []*corev1.Event
	for _, l := range m.listers {
		items, err := l.List(selector)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}

func (m *multiEventLister) Events(namespace string) corev1lister.EventNamespaceLister {
	nsl := make([]corev1lister.EventNamespaceLister, 0, len(m.listers))
	for _, l := range m.listers {
		nsl = append(nsl, l.Events(namespace))
	}
	return &multiEventNamespaceLister{listers: nsl}
}

type multiEventNamespaceLister struct {
	listers []corev1lister.EventNamespaceLister
}

func (m *multiEventNamespaceLister) List(selector labels.Selector) ([]*corev1.Event, error) {
	var all []*corev1.Event
	for _, l := range m.listers {
		items, err := l.List(selector)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}

func (m *multiEventNamespaceLister) Get(name string) (*corev1.Event, error) {
	for _, l := range m.listers {
		item, err := l.Get(name)
		if err == nil {
			return item, nil
		}
	}
	return nil, fmt.Errorf("event %q not found in any namespace lister", name)
}

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

type multiSecretLister struct {
	listers []corev1lister.SecretLister
}

func (m *multiSecretLister) List(selector labels.Selector) ([]*corev1.Secret, error) {
	var all []*corev1.Secret
	for _, l := range m.listers {
		items, err := l.List(selector)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}

func (m *multiSecretLister) Secrets(namespace string) corev1lister.SecretNamespaceLister {
	nsl := make([]corev1lister.SecretNamespaceLister, 0, len(m.listers))
	for _, l := range m.listers {
		nsl = append(nsl, l.Secrets(namespace))
	}
	return &multiSecretNamespaceLister{listers: nsl}
}

type multiSecretNamespaceLister struct {
	listers []corev1lister.SecretNamespaceLister
}

func (m *multiSecretNamespaceLister) List(selector labels.Selector) ([]*corev1.Secret, error) {
	var all []*corev1.Secret
	for _, l := range m.listers {
		items, err := l.List(selector)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}

func (m *multiSecretNamespaceLister) Get(name string) (*corev1.Secret, error) {
	for _, l := range m.listers {
		item, err := l.Get(name)
		if err == nil {
			return item, nil
		}
	}
	return nil, fmt.Errorf("secret %q not found in any namespace lister", name)
}

func (m *multiCronJobNamespaceLister) Get(name string) (*batchv1.CronJob, error) {
	for _, l := range m.listers {
		item, err := l.Get(name)
		if err == nil {
			return item, nil
		}
	}
	return nil, fmt.Errorf("cronjob %q not found in any namespace lister", name)
}

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
	return nil, fmt.Errorf("hpa %q not found in any namespace lister", name)
}
