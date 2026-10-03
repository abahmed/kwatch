//go:build e2e

package scenarios

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/util/retry"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

// workloadsPodSpec is a pod whose only container runs image with the
// local-only pull policy, so a missing image can never be pulled.
func workloadsPodSpec(name, image string, args ...string) corev1.PodSpec {
	container := corev1.Container{
		Name: name, Image: image, ImagePullPolicy: corev1.PullNever,
	}
	if len(args) > 0 {
		container.Command = append([]string{"/kwatch-e2e-workload"}, args...)
	}
	return corev1.PodSpec{Containers: []corev1.Container{container}}
}

func workloadsTemplate(
	app string, spec corev1.PodSpec,
) corev1.PodTemplateSpec {
	return corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": app}},
		Spec:       spec,
	}
}

func workloadsSelector(app string) *metav1.LabelSelector {
	return &metav1.LabelSelector{MatchLabels: map[string]string{"app": app}}
}

// workloadsStatefulSet never starts: its image is not on the node.
func workloadsStatefulSet(name string) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: name, Replicas: int32Ptr(1),
			Selector: workloadsSelector(name),
			Template: workloadsTemplate(name, workloadsPodSpec(
				name, "example.invalid/kwatch/missing:stateful")),
		},
	}
}

// workloadsDaemonSet never starts: its image is not on the node.
func workloadsDaemonSet(name string) *appsv1.DaemonSet {
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: appsv1.DaemonSetSpec{
			Selector: workloadsSelector(name),
			Template: workloadsTemplate(name, workloadsPodSpec(
				name, "example.invalid/kwatch/missing:daemon")),
		},
	}
}

// workloadsPDB requires one pod of the app to stay available.
func workloadsPDB(name, app string) *policyv1.PodDisruptionBudget {
	minAvailable := intstr.FromInt32(1)
	return &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable: &minAvailable, Selector: workloadsSelector(app),
		},
	}
}

// workloadsZeroPodQuota forbids creating any pod in its namespace.
func workloadsZeroPodQuota(name string) *corev1.ResourceQuota {
	return &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: corev1.ResourceQuotaSpec{Hard: corev1.ResourceList{
			corev1.ResourcePods: resource.MustParse("0"),
		}},
	}
}

// workloadsReplicaSet is healthy, so only the quota can stop it.
func workloadsReplicaSet(name string) *appsv1.ReplicaSet {
	return &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: appsv1.ReplicaSetSpec{
			Replicas: int32Ptr(1), Selector: workloadsSelector(name),
			Template: workloadsTemplate(name, workloadsPodSpec(
				"workload", workloadImage(), "healthy")),
		},
	}
}

// workloadsFailingJob fails at once and is never retried.
func workloadsFailingJob(name string) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: batchv1.JobSpec{
			BackoffLimit: int32Ptr(0),
			Template: corev1.PodTemplateSpec{Spec: func() corev1.PodSpec {
				spec := workloadsPodSpec("workload", workloadImage(), "crash")
				spec.RestartPolicy = corev1.RestartPolicyNever
				return spec
			}()},
		},
	}
}

// workloadsBreakRollout points the Deployment at a missing image with a
// short progress deadline. It re-reads the Deployment on every attempt,
// because the Deployment controller updates it concurrently.
func workloadsBreakRollout(
	ctx context.Context, e *harness.Environment, namespace, name string,
) error {
	deployments := e.Client.AppsV1().Deployments(namespace)
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		deployment, err := deployments.Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		deployment.Spec.Template.Spec.Containers[0].Image =
			"example.invalid/kwatch/missing:rollout"
		deployment.Spec.ProgressDeadlineSeconds = int32Ptr(5)
		_, err = deployments.Update(ctx, deployment, metav1.UpdateOptions{})
		return err
	})
}
