package kube_test

import (
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/abahmed/kwatch/internal/knowledge"
)

const testNamespace = "testns"

func pod(name string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: testNamespace, UID: types.UID(name),
			ManagedFields: []metav1.ManagedFieldsEntry{
				{Manager: "kubectl", Time: timePtr(fixedTime())},
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app", Image: "app:1.0"},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{
					Type: corev1.PodReady, Status: corev1.ConditionTrue,
					LastTransitionTime: metav1.NewTime(fixedTime()),
				},
			},
		},
	}
}

func node(name string) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, UID: types.UID(name),
			ManagedFields: []metav1.ManagedFieldsEntry{
				{Manager: "kubelet", Time: timePtr(fixedTime())},
			},
		},
		Spec: corev1.NodeSpec{},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type: corev1.NodeReady, Status: corev1.ConditionTrue,
					LastTransitionTime: metav1.NewTime(fixedTime()),
				},
			},
			NodeInfo: corev1.NodeSystemInfo{
				KubeletVersion:          "v1.25.0",
				ContainerRuntimeVersion: "docker://20.10.0",
				KernelVersion:           "5.10.0",
				OperatingSystem:         "linux",
			},
		},
	}
}

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

func service(name string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: testNamespace, UID: types.UID(name),
			ManagedFields: []metav1.ManagedFieldsEntry{
				{Manager: "kubectl", Time: timePtr(fixedTime())},
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Selector: map[string]string{
				"app": "test",
			},
			Ports: []corev1.ServicePort{
				{Port: 8080, Protocol: corev1.ProtocolTCP},
			},
		},
	}
}

func secret(name string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: testNamespace, UID: types.UID(name),
			ManagedFields: []metav1.ManagedFieldsEntry{
				{Manager: "kubectl", Time: timePtr(fixedTime())},
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"key1": []byte("value1"),
		},
	}
}

func configMap(name string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: testNamespace, UID: types.UID(name),
			ManagedFields: []metav1.ManagedFieldsEntry{
				{Manager: "kubectl", Time: timePtr(fixedTime())},
			},
		},
		Data: map[string]string{
			"config.yaml": "data",
		},
	}
}

func pvc(name string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: testNamespace, UID: types.UID(name),
			ManagedFields: []metav1.ManagedFieldsEntry{
				{Manager: "kubectl", Time: timePtr(fixedTime())},
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: mustQuantity("10Gi"),
				},
			},
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: corev1.ClaimBound,
		},
	}
}

func pv(name string) *corev1.PersistentVolume {
	return &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, UID: types.UID(name),
			ManagedFields: []metav1.ManagedFieldsEntry{
				{Manager: "kubelet", Time: timePtr(fixedTime())},
			},
		},
		Spec: corev1.PersistentVolumeSpec{
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: mustQuantity("10Gi"),
			},
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
		},
		Status: corev1.PersistentVolumeStatus{
			Phase: corev1.VolumeBound,
		},
	}
}

func storageClass(name string) *storagev1.StorageClass {
	return &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, UID: types.UID(name),
			ManagedFields: []metav1.ManagedFieldsEntry{
				{Manager: "system", Time: timePtr(fixedTime())},
			},
		},
		Provisioner: "ebs.csi.aws.com",
	}
}

func endpointSlice(name string) *discoveryv1.EndpointSlice {
	return &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: testNamespace, UID: types.UID(name),
			Labels: map[string]string{
				discoveryv1.LabelServiceName: "mysvc",
			},
			ManagedFields: []metav1.ManagedFieldsEntry{
				{Manager: "controller", Time: timePtr(fixedTime())},
			},
		},
		Endpoints: []discoveryv1.Endpoint{
			{
				Addresses: []string{"10.0.0.1"},
				Conditions: discoveryv1.EndpointConditions{
					Ready: boolPtr(true),
				},
				TargetRef: &corev1.ObjectReference{
					Kind: "Pod", Name: "pod1", Namespace: testNamespace,
				},
			},
		},
	}
}

func ingress(name string) *networkingv1.Ingress {
	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: testNamespace, UID: types.UID(name),
			ManagedFields: []metav1.ManagedFieldsEntry{
				{Manager: "nginx", Time: timePtr(fixedTime())},
			},
		},
		Spec: networkingv1.IngressSpec{
			DefaultBackend: &networkingv1.IngressBackend{
				Service: &networkingv1.IngressServiceBackend{
					Name: "mysvc",
					Port: networkingv1.ServiceBackendPort{Number: 8080},
				},
			},
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

func fixedTime() time.Time {
	return time.Date(2025, 1, 15, 10, 30, 45, 0, time.UTC)
}

func timePtr(t time.Time) *metav1.Time {
	mt := metav1.NewTime(t)
	return &mt
}

func boolPtr(b bool) *bool {
	return &b
}

func mustQuantity(s string) resource.Quantity {
	q, _ := resource.ParseQuantity(s)
	return q
}

// factsByKind groups facts by kind for easier assertion.
func factsByKind(
	facts []knowledge.Fact,
) map[knowledge.FactKind][]knowledge.Fact {
	out := make(map[knowledge.FactKind][]knowledge.Fact)
	for _, f := range facts {
		out[f.Kind] = append(out[f.Kind], f)
	}
	return out
}

// factsByEntity groups facts by entity for easier assertion.
func factsByEntity(facts []knowledge.Fact) map[string][]knowledge.Fact {
	out := make(map[string][]knowledge.Fact)
	for _, f := range facts {
		key := f.Entity.String()
		out[key] = append(out[key], f)
	}
	return out
}
