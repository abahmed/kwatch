//go:build e2e

package scenarios

import (
	"context"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

func TestScenarioPodLifecycleHookFailure(t *testing.T) {
	runScenario(t, "pod.lifecycle-hook", func(
		ctx context.Context,
		t *testing.T,
		e *harness.Environment,
	) {
		namespace := uniqueNamespace(t.Name())
		if err := createNamespace(ctx, e, namespace); err != nil {
			t.Fatal(err)
		}
		defer cleanupNamespace(t, e, namespace)
		_, err := e.Client.CoreV1().Pods(namespace).Create(ctx, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "post-start"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{
				Name: "workload", Image: workloadImage(),
				Command:         []string{"/kwatch-e2e-workload", "sleep"},
				ImagePullPolicy: corev1.PullNever,
				Lifecycle: &corev1.Lifecycle{PostStart: &corev1.LifecycleHandler{
					Exec: &corev1.ExecAction{
						Command: []string{
							"/kwatch-e2e-workload", "startup-error",
						},
					},
				}},
			}}},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.Audit.WaitFor(ctx, harness.AuditMatch{
			Namespace: namespace, Resource: "post-start",
			Reason: "PostStartHookError", Count: 1,
		}); err != nil {
			t.Fatal(err)
		}
	})
}

func TestScenarioCronJobSuspended(t *testing.T) {
	runScenario(t, "workload.cronjob", func(
		ctx context.Context,
		t *testing.T,
		e *harness.Environment,
	) {
		namespace := uniqueNamespace(t.Name())
		if err := createNamespace(ctx, e, namespace); err != nil {
			t.Fatal(err)
		}
		defer cleanupNamespace(t, e, namespace)
		suspended := true
		_, err := e.Client.BatchV1().CronJobs(namespace).Create(ctx,
			&batchv1.CronJob{
				ObjectMeta: metav1.ObjectMeta{Name: "suspended"},
				Spec: batchv1.CronJobSpec{
					Schedule: "*/5 * * * *", Suspend: &suspended,
					JobTemplate: batchv1.JobTemplateSpec{Spec: batchv1.JobSpec{
						Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyNever,
							Containers: []corev1.Container{{
								Name: "workload", Image: workloadImage(),
								Command: []string{
									"/kwatch-e2e-workload", "healthy",
								},
								ImagePullPolicy: corev1.PullNever,
							}},
						}},
					}},
				},
			}, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.Audit.WaitFor(ctx, harness.AuditMatch{
			Namespace: namespace, Resource: "suspended",
			Reason: "CronJobSuspended", Count: 1,
		}); err != nil {
			t.Fatal(err)
		}
	})
}

func TestScenarioMissingRequiredReferences(t *testing.T) {
	runScenario(t, "security.secret-reference", func(
		ctx context.Context,
		t *testing.T,
		e *harness.Environment,
	) {
		namespace := uniqueNamespace(t.Name())
		if err := createNamespace(ctx, e, namespace); err != nil {
			t.Fatal(err)
		}
		defer cleanupNamespace(t, e, namespace)
		_, err := e.Client.CoreV1().Pods(namespace).Create(ctx, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "missing-references"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{
				Name: "workload", Image: workloadImage(),
				Command:         []string{"/kwatch-e2e-workload", "sleep"},
				ImagePullPolicy: corev1.PullNever,
				EnvFrom: []corev1.EnvFromSource{{
					SecretRef: &corev1.SecretEnvSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "missing-secret",
						},
					},
				}},
			}}},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.Audit.WaitFor(ctx, harness.AuditMatch{
			Namespace: namespace, Resource: "missing-references",
			Reason: "ProjectedSecretMissing", Count: 1,
		}); err != nil {
			t.Fatal(err)
		}
	})
}

func TestScenarioMissingConfigMapReference(t *testing.T) {
	runScenario(t, "security.configmap-reference", func(
		ctx context.Context,
		t *testing.T,
		e *harness.Environment,
	) {
		namespace := uniqueNamespace(t.Name())
		if err := createNamespace(ctx, e, namespace); err != nil {
			t.Fatal(err)
		}
		defer cleanupNamespace(t, e, namespace)
		_, err := e.Client.CoreV1().Pods(namespace).Create(ctx, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "missing-configmap"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{
				Name: "workload", Image: workloadImage(),
				Command:         []string{"/kwatch-e2e-workload", "sleep"},
				ImagePullPolicy: corev1.PullNever,
				EnvFrom: []corev1.EnvFromSource{{
					ConfigMapRef: &corev1.ConfigMapEnvSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "missing-configmap",
						},
					},
				}},
			}}},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.Audit.WaitFor(ctx, harness.AuditMatch{
			Namespace: namespace, Resource: "missing-configmap",
			Reason: "ProjectedConfigMapMissing", Count: 1,
		}); err != nil {
			t.Fatal(err)
		}
	})
}

func TestScenarioMissingServiceAccountReference(t *testing.T) {
	runScenario(t, "security.rbac", func(
		ctx context.Context,
		t *testing.T,
		e *harness.Environment,
	) {
		namespace := uniqueNamespace(t.Name())
		if err := createNamespace(ctx, e, namespace); err != nil {
			t.Fatal(err)
		}
		defer cleanupNamespace(t, e, namespace)
		_, err := e.Client.CoreV1().Pods(namespace).Create(ctx, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "missing-service-account"},
			Spec: corev1.PodSpec{
				ServiceAccountName: "missing-service-account",
				Containers: []corev1.Container{{
					Name: "workload", Image: workloadImage(),
					Command:         []string{"/kwatch-e2e-workload", "sleep"},
					ImagePullPolicy: corev1.PullNever,
				}},
			},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.Audit.WaitFor(ctx, harness.AuditMatch{
			Namespace: namespace, Resource: "missing-service-account",
			Reason: "ServiceAccountMissing", Count: 1,
		}); err != nil {
			t.Fatal(err)
		}
	})
}

func TestScenarioMissingIngressBackend(t *testing.T) {
	runScenario(t, "networking.ingress", func(
		ctx context.Context,
		t *testing.T,
		e *harness.Environment,
	) {
		namespace := uniqueNamespace(t.Name())
		if err := createNamespace(ctx, e, namespace); err != nil {
			t.Fatal(err)
		}
		defer cleanupNamespace(t, e, namespace)
		pathType := networkingv1.PathTypePrefix
		_, err := e.Client.NetworkingV1().Ingresses(namespace).Create(ctx,
			&networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{Name: "missing-backend"},
				Spec: networkingv1.IngressSpec{Rules: []networkingv1.IngressRule{{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{{
							Path: "/", PathType: &pathType,
							Backend: networkingv1.IngressBackend{
								Service: &networkingv1.IngressServiceBackend{
									Name: "missing-service",
									Port: networkingv1.ServiceBackendPort{
										Number: 8080,
									},
								},
							},
						}}},
				}}},
			}, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		waitCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		if _, err := e.Audit.WaitFor(waitCtx, harness.AuditMatch{
			Namespace: namespace, Resource: "missing-backend",
			Reason: "IngressBackendNotFound", Count: 1,
		}); err != nil {
			t.Fatal(err)
		}
	})
}

func TestScenarioRestrictiveNetworkPolicy(t *testing.T) {
	runScenario(t, "networking.network-policy", func(
		ctx context.Context,
		t *testing.T,
		e *harness.Environment,
	) {
		namespace := uniqueNamespace(t.Name())
		if err := createNamespace(ctx, e, namespace); err != nil {
			t.Fatal(err)
		}
		defer cleanupNamespace(t, e, namespace)
		_, err := e.Client.NetworkingV1().NetworkPolicies(namespace).Create(
			ctx, &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "deny-egress"},
				Spec: networkingv1.NetworkPolicySpec{
					PodSelector: metav1.LabelSelector{},
					PolicyTypes: []networkingv1.PolicyType{
						networkingv1.PolicyTypeEgress,
					},
					Egress: []networkingv1.NetworkPolicyEgressRule{},
				},
			}, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.Audit.WaitFor(ctx, harness.AuditMatch{
			Namespace: namespace, Resource: "deny-egress",
			Reason: "RestrictiveNetworkPolicy", Count: 1,
		}); err != nil {
			t.Fatal(err)
		}
	})
}

func TestScenarioPodSecurityAdmission(t *testing.T) {
	runScenario(t, "security.pod-security-admission", func(
		ctx context.Context,
		t *testing.T,
		e *harness.Environment,
	) {
		namespace := uniqueNamespace(t.Name())
		_, err := e.Client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
				Labels: map[string]string{
					"pod-security.kubernetes.io/enforce": "restricted",
				},
			},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		defer cleanupNamespace(t, e, namespace)
		privileged := true
		_, err = e.Client.CoreV1().Pods(namespace).Create(ctx, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "privileged"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{
				Name: "workload", Image: workloadImage(),
				Command:         []string{"/kwatch-e2e-workload", "sleep"},
				ImagePullPolicy: corev1.PullNever,
				SecurityContext: &corev1.SecurityContext{
					Privileged: &privileged,
				},
			}}},
		}, metav1.CreateOptions{})
		if !apierrors.IsForbidden(err) {
			t.Fatalf("expected Pod Security Admission rejection, got %v", err)
		}
		if err := e.AssertHealthy(ctx); err != nil {
			t.Fatal(err)
		}
	})
}
