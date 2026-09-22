//go:build e2e

package scenarios

import (
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

func TestScenarioResolution(t *testing.T) {
	runScenario(t, "lifecycle.resolution", func(
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
			"recovery", "crash")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.Audit.WaitFor(ctx, harness.AuditMatch{
			Namespace: namespace,
			Resource:  "recovery",
			Reason:    "CrashLoopBackOff",
			Action:    "create",
			Count:     1,
		}); err != nil {
			t.Fatal(err)
		}
		deployment.Spec.Template.Spec.Containers[0].Command = []string{
			"/kwatch-e2e-workload", "healthy",
		}
		if _, err := e.Client.AppsV1().Deployments(namespace).Update(
			ctx, deployment, metav1.UpdateOptions{},
		); err != nil {
			t.Fatal(err)
		}
		waitCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		if err := e.WaitForDeployment(waitCtx, namespace, "recovery"); err != nil {
			t.Fatal(err)
		}
		entries, err := e.Audit.WaitFor(ctx, harness.AuditMatch{
			Namespace: namespace,
			Resource:  "recovery",
			Reason:    "CrashLoopBackOff",
			Action:    "resolved",
			Count:     1,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 {
			t.Fatalf("expected one resolution, got %d", len(entries))
		}
	})
}

func TestScenarioRefailureAfterRecovery(t *testing.T) {
	runScenario(t, "pod.re-failure-after-recovery", func(
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
			"refailure", "crash")
		if err != nil {
			t.Fatal(err)
		}
		match := harness.AuditMatch{
			Namespace: namespace, Resource: "refailure",
			Reason: "CrashLoopBackOff", Action: "create", Count: 1,
		}
		if _, err := e.Audit.WaitFor(ctx, match); err != nil {
			t.Fatal(err)
		}
		deployment.Spec.Template.Spec.Containers[0].Command = []string{
			"/kwatch-e2e-workload", "healthy",
		}
		deployment, err = e.Client.AppsV1().Deployments(namespace).Update(
			ctx, deployment, metav1.UpdateOptions{},
		)
		if err != nil {
			t.Fatal(err)
		}
		waitCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		if err := e.WaitForDeployment(waitCtx, namespace, "refailure"); err != nil {
			t.Fatal(err)
		}
		if _, err := e.Audit.WaitFor(ctx, harness.AuditMatch{
			Namespace: namespace, Resource: "refailure",
			Reason: "CrashLoopBackOff", Action: "resolved", Count: 1,
		}); err != nil {
			t.Fatal(err)
		}
		deployment.Spec.Template.Spec.Containers[0].Command = []string{
			"/kwatch-e2e-workload", "crash",
		}
		if _, err := e.Client.AppsV1().Deployments(namespace).Update(
			ctx, deployment, metav1.UpdateOptions{},
		); err != nil {
			t.Fatal(err)
		}
		entries, err := e.Audit.WaitFor(ctx, match)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 2 {
			t.Fatalf("expected two create transitions, got %d", len(entries))
		}
	})
}

func TestScenarioRestartPersistence(t *testing.T) {
	runScenario(t, "lifecycle.restart-persistence", func(
		ctx context.Context,
		t *testing.T,
		e *harness.Environment,
	) {
		namespace := uniqueNamespace(t.Name())
		if err := e.Receiver.Clear(ctx); err != nil {
			t.Fatal(err)
		}
		if err := createNamespace(ctx, e, namespace); err != nil {
			t.Fatal(err)
		}
		defer cleanupNamespace(t, e, namespace)
		if _, err := createLifecycleDeployment(ctx, e, namespace,
			"persistent", "crash"); err != nil {
			t.Fatal(err)
		}
		if _, err := e.Audit.WaitFor(ctx, harness.AuditMatch{
			Namespace: namespace,
			Resource:  "persistent",
			Reason:    "CrashLoopBackOff",
			Action:    "create",
			Count:     1,
		}); err != nil {
			t.Fatal(err)
		}
		deliveryMatch := harness.DeliveryMatch{
			Name: "persistent", Reason: "CrashLoopBackOff",
		}
		if _, err := e.Receiver.WaitForMatchCount(
			ctx, deliveryMatch, 1,
		); err != nil {
			t.Fatal(err)
		}
		leader, err := e.WaitForLeaseHolder(ctx, "kwatch", "kwatch-leader")
		if err != nil {
			t.Fatal(err)
		}
		if err := e.Client.CoreV1().Pods("kwatch").Delete(
			ctx, leader, metav1.DeleteOptions{},
		); err != nil {
			t.Fatal(err)
		}
		waitCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		if _, err := e.WaitForLeaseChange(
			waitCtx, "kwatch", "kwatch-leader", leader,
		); err != nil {
			t.Fatal(err)
		}
		entries, err := e.Receiver.Matching(ctx, deliveryMatch)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 {
			t.Fatalf("restart changed delivery count: %d", len(entries))
		}
		if err := e.AssertHealthy(ctx); err != nil {
			t.Fatal(err)
		}
	})
}

func TestScenarioLeaderFailover(t *testing.T) {
	runScenario(t, "lifecycle.leader-failover", func(
		ctx context.Context,
		t *testing.T,
		e *harness.Environment,
	) {
		oldLeader, err := e.WaitForLeaseHolder(ctx, "kwatch", "kwatch-leader")
		if err != nil {
			t.Fatal(err)
		}
		if err := e.Client.CoreV1().Pods("kwatch").Delete(
			ctx, oldLeader, metav1.DeleteOptions{},
		); err != nil {
			t.Fatal(err)
		}
		waitCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		newLeader, err := e.WaitForLeaseChange(
			waitCtx, "kwatch", "kwatch-leader", oldLeader,
		)
		if err != nil {
			t.Fatal(err)
		}
		if newLeader == oldLeader {
			t.Fatalf("leader did not change from %q", oldLeader)
		}
		if err := e.Health.AssertOK(ctx, "/availabilityz"); err != nil {
			t.Fatal(err)
		}
		namespace := uniqueNamespace(t.Name())
		if err := createNamespace(ctx, e, namespace); err != nil {
			t.Fatal(err)
		}
		defer cleanupNamespace(t, e, namespace)
		if _, err := createLifecycleDeployment(ctx, e, namespace,
			"takeover", "crash"); err != nil {
			t.Fatal(err)
		}
		entries, err := e.Audit.WaitFor(ctx, harness.AuditMatch{
			Namespace: namespace,
			Resource:  "takeover",
			Reason:    "CrashLoopBackOff",
			Action:    "create",
			Count:     1,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 {
			t.Fatalf("expected one post-failover incident, got %d", len(entries))
		}
	})
}

func createLifecycleDeployment(
	ctx context.Context,
	e *harness.Environment,
	namespace, name, command string,
) (*appsv1.Deployment, error) {
	return e.Client.AppsV1().Deployments(namespace).Create(ctx,
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec: appsv1.DeploymentSpec{
				Replicas: int32Ptr(1),
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{
					"app": name,
				}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
						"app": name,
					}},
					Spec: corev1.PodSpec{Containers: []corev1.Container{{
						Name:  "workload",
						Image: workloadImage(),
						Command: []string{
							"/kwatch-e2e-workload", command,
						},
						ImagePullPolicy: corev1.PullNever,
					}}},
				},
			},
		}, metav1.CreateOptions{})
}
