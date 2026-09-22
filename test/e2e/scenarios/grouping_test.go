//go:build e2e

package scenarios

import (
	"context"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

func TestScenarioGroupingSameErrorManyOwners(t *testing.T) {
	runScenario(t, "grouping.same-error-many-owners", func(
		ctx context.Context,
		t *testing.T,
		e *harness.Environment,
	) {
		namespace := uniqueNamespace(t.Name())
		if err := createNamespace(ctx, e, namespace); err != nil {
			t.Fatal(err)
		}
		defer cleanupNamespace(t, e, namespace)
		if err := e.Receiver.Clear(ctx); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"api", "worker", "scheduler"} {
			if err := createFailingDeployment(ctx, e, namespace, name); err != nil {
				t.Fatal(err)
			}
		}
		waitCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		if err := e.WaitForPodCount(waitCtx, namespace, func(
			pods []corev1.Pod,
		) bool {
			if len(pods) != 3 {
				return false
			}
			for _, pod := range pods {
				if !harness.PodHasReason(&pod, "CrashLoopBackOff") {
					return false
				}
			}
			return true
		}); err != nil {
			t.Fatal(err)
		}
		entries, err := e.Audit.WaitFor(ctx, harness.AuditMatch{
			Namespace: namespace,
			Reason:    "CrashLoopBackOff",
			Count:     1,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 {
			t.Fatalf("expected one grouped audit entry, got %d", len(entries))
		}
		requests, err := e.Receiver.WaitForCount(ctx, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(requests) != 1 {
			t.Fatalf("expected one grouped delivery, got %d", len(requests))
		}
		body := string(requests[0].Body)
		for _, owner := range []string{"api", "worker", "scheduler"} {
			if !strings.Contains(body, owner) {
				t.Fatalf("grouped delivery omitted owner %q", owner)
			}
		}
	})
}

func TestScenarioGroupingManyReplicasOneOwner(t *testing.T) {
	runScenario(t, "grouping.same-error-one-owner", func(
		ctx context.Context,
		t *testing.T,
		e *harness.Environment,
	) {
		namespace := uniqueNamespace(t.Name())
		if err := createNamespace(ctx, e, namespace); err != nil {
			t.Fatal(err)
		}
		defer cleanupNamespace(t, e, namespace)
		if err := e.Receiver.Clear(ctx); err != nil {
			t.Fatal(err)
		}
		if err := createFailingDeploymentReplicas(
			ctx, e, namespace, "replicas", 3,
		); err != nil {
			t.Fatal(err)
		}
		waitCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		if err := e.WaitForPodCount(waitCtx, namespace, func(
			pods []corev1.Pod,
		) bool {
			if len(pods) != 3 {
				return false
			}
			for _, pod := range pods {
				if !harness.PodHasReason(&pod, "CrashLoopBackOff") {
					return false
				}
			}
			return true
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := e.Audit.WaitFor(ctx, harness.AuditMatch{
			Namespace: namespace, Resource: "replicas",
			Reason: "CrashLoopBackOff", Count: 1,
		}); err != nil {
			t.Fatal(err)
		}
		requests, err := e.Receiver.WaitForCount(ctx, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(requests) != 1 {
			t.Fatalf("replicas produced %d deliveries", len(requests))
		}
	})
}

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
					Spec: corev1.PodSpec{Containers: []corev1.Container{{
						Name:            "workload",
						Image:           workloadImage(),
						Command:         []string{"/kwatch-e2e-workload", "crash"},
						ImagePullPolicy: corev1.PullNever,
					}}},
				},
			},
		}, metav1.CreateOptions{})
	return err
}

func int32Ptr(value int32) *int32 {
	return &value
}
