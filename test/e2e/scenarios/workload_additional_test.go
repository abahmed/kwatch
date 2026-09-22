//go:build e2e

package scenarios

import (
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

func intOrStringPtr(value int32) *intstr.IntOrString {
	result := intstr.FromInt32(value)
	return &result
}

func TestScenarioStatefulSetFailure(t *testing.T) {
	runScenario(t, "workload.statefulset", func(
		ctx context.Context,
		t *testing.T,
		e *harness.Environment,
	) {
		namespace := uniqueNamespace(t.Name())
		if err := createNamespace(ctx, e, namespace); err != nil {
			t.Fatal(err)
		}
		defer cleanupNamespace(t, e, namespace)
		replicas := int32(1)
		_, err := e.Client.AppsV1().StatefulSets(namespace).Create(ctx,
			&appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "database"},
				Spec: appsv1.StatefulSetSpec{
					ServiceName: "database",
					Replicas:    &replicas,
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{
						"app": "database",
					}},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
							"app": "database",
						}},
						Spec: corev1.PodSpec{Containers: []corev1.Container{{
							Name:            "database",
							Image:           "example.invalid/kwatch/missing:stateful",
							ImagePullPolicy: corev1.PullNever,
						}}},
					},
				},
			}, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.Audit.WaitFor(ctx, harness.AuditMatch{
			Namespace: namespace, Resource: "database",
			Reason: "StsUnavailable", Count: 1,
		}); err != nil {
			t.Fatal(err)
		}
	})
}

func TestScenarioDaemonSetFailure(t *testing.T) {
	runScenario(t, "workload.daemonset", func(
		ctx context.Context,
		t *testing.T,
		e *harness.Environment,
	) {
		namespace := uniqueNamespace(t.Name())
		if err := createNamespace(ctx, e, namespace); err != nil {
			t.Fatal(err)
		}
		defer cleanupNamespace(t, e, namespace)
		_, err := e.Client.AppsV1().DaemonSets(namespace).Create(ctx,
			&appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{Name: "agent"},
				Spec: appsv1.DaemonSetSpec{
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{
						"app": "agent",
					}},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
							"app": "agent",
						}},
						Spec: corev1.PodSpec{Containers: []corev1.Container{{
							Name:            "agent",
							Image:           "example.invalid/kwatch/missing:daemon",
							ImagePullPolicy: corev1.PullNever,
						}}},
					},
				},
			}, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.Audit.WaitFor(ctx, harness.AuditMatch{
			Namespace: namespace, Resource: "agent",
			Reason: "DaemonSetUnavailable", Count: 1,
		}); err != nil {
			t.Fatal(err)
		}
	})
}

func TestScenarioPDBDisruption(t *testing.T) {
	runScenario(t, "workload.pdb-disruption", func(
		ctx context.Context,
		t *testing.T,
		e *harness.Environment,
	) {
		namespace := uniqueNamespace(t.Name())
		if err := createNamespace(ctx, e, namespace); err != nil {
			t.Fatal(err)
		}
		defer cleanupNamespace(t, e, namespace)
		deployment, err := createLifecycleDeployment(ctx, e, namespace,
			"protected", "crash")
		if err != nil {
			t.Fatal(err)
		}
		_, err = e.Client.PolicyV1().PodDisruptionBudgets(namespace).Create(
			ctx, &policyv1.PodDisruptionBudget{
				ObjectMeta: metav1.ObjectMeta{Name: "protected-budget"},
				Spec: policyv1.PodDisruptionBudgetSpec{
					MinAvailable: intOrStringPtr(1),
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{
						"app": deployment.Name,
					}},
				},
			}, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		waitCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		if _, err := e.Audit.WaitFor(waitCtx, harness.AuditMatch{
			Namespace: namespace, Resource: "protected-budget",
			Reason: "PdbViolation", Count: 1,
		}); err != nil {
			t.Fatal(err)
		}
	})
}

func TestScenarioReplicaSetFailure(t *testing.T) {
	runScenario(t, "workload.replicaset", func(
		ctx context.Context,
		t *testing.T,
		e *harness.Environment,
	) {
		namespace := uniqueNamespace(t.Name())
		if err := createNamespace(ctx, e, namespace); err != nil {
			t.Fatal(err)
		}
		defer cleanupNamespace(t, e, namespace)
		_, err := e.Client.CoreV1().ResourceQuotas(namespace).Create(ctx,
			&corev1.ResourceQuota{
				ObjectMeta: metav1.ObjectMeta{Name: "zero-pods"},
				Spec: corev1.ResourceQuotaSpec{Hard: corev1.ResourceList{
					corev1.ResourcePods: resource.MustParse("0"),
				}},
			}, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		_, err = e.Client.AppsV1().ReplicaSets(namespace).Create(ctx,
			&appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{Name: "blocked"},
				Spec: appsv1.ReplicaSetSpec{
					Replicas: int32Ptr(1),
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{
						"app": "blocked",
					}},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
							"app": "blocked",
						}},
						Spec: corev1.PodSpec{Containers: []corev1.Container{{
							Name: "workload", Image: workloadImage(),
							Command: []string{
								"/kwatch-e2e-workload", "healthy",
							},
							ImagePullPolicy: corev1.PullNever,
						}}},
					},
				},
			}, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.Audit.WaitFor(ctx, harness.AuditMatch{
			Namespace: namespace, Resource: "blocked",
			Reason: "ReplicaSetFailure", Count: 1,
		}); err != nil {
			t.Fatal(err)
		}
	})
}
