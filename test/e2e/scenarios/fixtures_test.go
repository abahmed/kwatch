//go:build e2e

package scenarios

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

func createFailingDeployment(
	ctx context.Context,
	e *harness.Environment,
	namespace, name string,
) error {
	return createFailingDeploymentReplicas(ctx, e, namespace, name, 1)
}

func createFailingDeploymentReplicas(
	ctx context.Context,
	e *harness.Environment,
	namespace, name string,
	replicas int32,
) error {
	return createDeployment(ctx, e, namespace, name, replicas,
		corev1.Container{
			Name:            "workload",
			Image:           workloadImage(),
			Command:         []string{"/kwatch-e2e-workload", "crash"},
			ImagePullPolicy: corev1.PullNever,
		}, nil)
}

// createDeployment builds a Deployment around one container; mutate may
// adjust the Pod spec, for example to pin the workload to a node.
func createDeployment(
	ctx context.Context,
	e *harness.Environment,
	namespace, name string,
	replicas int32,
	container corev1.Container,
	mutate func(*corev1.PodSpec),
) error {
	spec := corev1.PodSpec{Containers: []corev1.Container{container}}
	if mutate != nil {
		mutate(&spec)
	}
	_, err := e.Client.AppsV1().Deployments(namespace).Create(ctx,
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec: appsv1.DeploymentSpec{
				Replicas: int32Ptr(replicas),
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{
					"app": name,
				}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
						"app": name,
					}},
					Spec: spec,
				},
			},
		}, metav1.CreateOptions{})
	return err
}

func int32Ptr(value int32) *int32 {
	return &value
}
