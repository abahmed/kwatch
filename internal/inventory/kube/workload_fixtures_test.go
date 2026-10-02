package kube_test

import (
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func deployment(name string) *appsv1.Deployment {
	replicas := int32(3)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: testNamespace, UID: types.UID(name),
			Generation: 5,
			ManagedFields: []metav1.ManagedFieldsEntry{
				{Manager: "kubectl", Time: timePtr(fixedTime())},
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "app", Image: "app:1.0"},
					},
				},
			},
		},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 5,
			Replicas:           3,
			UpdatedReplicas:    3,
			ReadyReplicas:      3,
		},
	}
}

func cronJob(name string) *batchv1.CronJob {
	return &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: testNamespace, UID: types.UID(name),
			ManagedFields: []metav1.ManagedFieldsEntry{
				{Manager: "kubectl", Time: timePtr(fixedTime())},
			},
		},
		Spec: batchv1.CronJobSpec{
			Schedule: "0 * * * *",
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{Name: "job", Image: "job:1.0"},
							},
						},
					},
				},
			},
		},
	}
}

func hpa(name string) *autoscalingv2.HorizontalPodAutoscaler {
	minReplicas := int32(1)
	maxReplicas := int32(10)
	return &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: testNamespace, UID: types.UID(name),
			ManagedFields: []metav1.ManagedFieldsEntry{
				{Manager: "kubectl", Time: timePtr(fixedTime())},
			},
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "deploy",
			},
			MinReplicas: &minReplicas,
			MaxReplicas: maxReplicas,
		},
		Status: autoscalingv2.HorizontalPodAutoscalerStatus{
			CurrentReplicas: 3,
			DesiredReplicas: 3,
		},
	}
}

func replicaSet(name string) *appsv1.ReplicaSet {
	replicas := int32(2)
	return &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: testNamespace, UID: types.UID(name),
			Generation: 1,
			ManagedFields: []metav1.ManagedFieldsEntry{
				{Manager: "kubectl", Time: timePtr(fixedTime())},
			},
		},
		Spec: appsv1.ReplicaSetSpec{
			Replicas: &replicas,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "app", Image: "app:1.0"},
					},
				},
			},
		},
	}
}

func statefulSet(name string) *appsv1.StatefulSet {
	replicas := int32(3)
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: testNamespace, UID: types.UID(name),
			ManagedFields: []metav1.ManagedFieldsEntry{
				{Manager: "kubectl", Time: timePtr(fixedTime())},
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "app", Image: "app:1.0"},
					},
				},
			},
		},
	}
}

func daemonSet(name string) *appsv1.DaemonSet {
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: testNamespace, UID: types.UID(name),
			ManagedFields: []metav1.ManagedFieldsEntry{
				{Manager: "kubectl", Time: timePtr(fixedTime())},
			},
		},
		Spec: appsv1.DaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "agent", Image: "agent:1.0"},
					},
				},
			},
		},
		Status: appsv1.DaemonSetStatus{
			DesiredNumberScheduled: 3,
			CurrentNumberScheduled: 3,
			NumberReady:            3,
		},
	}
}

func job(name string) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: testNamespace, UID: types.UID(name),
			ManagedFields: []metav1.ManagedFieldsEntry{
				{Manager: "kubectl", Time: timePtr(fixedTime())},
			},
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "task", Image: "task:1.0"},
					},
				},
			},
		},
		Status: batchv1.JobStatus{
			Succeeded: 5,
		},
	}
}
