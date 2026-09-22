//go:build e2e

package scenarios

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"math/big"
	"testing"
	"time"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

func TestScenarioExtendedTLS(t *testing.T) {
	runExtendedScenario(t, "integration.tls", func(
		ctx context.Context,
		t *testing.T,
		e *harness.Environment,
	) {
		namespace := uniqueNamespace(t.Name())
		if err := createNamespace(ctx, e, namespace); err != nil {
			t.Fatal(err)
		}
		defer cleanupNamespace(t, e, namespace)
		_, err := e.Client.CoreV1().Secrets(namespace).Create(ctx,
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "expired-tls"},
				Type:       corev1.SecretTypeTLS,
				Data: map[string][]byte{
					"tls.crt": expiredCertificate(t),
					"tls.key": []byte("not-used-by-monitor"),
				},
			}, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.Audit.WaitFor(ctx, harness.AuditMatch{
			Namespace: namespace, Resource: "expired-tls",
			Reason: "TLSCertExpired", Count: 1,
		}); err != nil {
			t.Fatal(err)
		}
	})
}

func TestScenarioExtendedAdmissionWebhook(t *testing.T) {
	runExtendedScenario(t, "security.admission-webhook", func(
		ctx context.Context,
		t *testing.T,
		e *harness.Environment,
	) {
		namespace := uniqueNamespace(t.Name())
		if err := createNamespace(ctx, e, namespace); err != nil {
			t.Fatal(err)
		}
		defer cleanupNamespace(t, e, namespace)
		failurePolicy := admissionregistrationv1.Ignore
		sideEffects := admissionregistrationv1.SideEffectClassNone
		_, err := e.Client.AdmissionregistrationV1().
			ValidatingWebhookConfigurations().Create(ctx,
			&admissionregistrationv1.ValidatingWebhookConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name: "kwatch-e2e-missing-webhook",
				},
				Webhooks: []admissionregistrationv1.
					ValidatingWebhook{{
					Name: "missing.kwatch.e2e",
					ClientConfig: admissionregistrationv1.
						WebhookClientConfig{Service: &admissionregistrationv1.
						ServiceReference{
						Name:      "missing-webhook",
						Namespace: namespace,
					}},
					AdmissionReviewVersions: []string{"v1"},
					FailurePolicy:           &failurePolicy,
					SideEffects:             &sideEffects,
				}},
			}, metav1.CreateOptions{},
		)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			_ = e.Client.AdmissionregistrationV1().
				ValidatingWebhookConfigurations().Delete(
				context.Background(), "kwatch-e2e-missing-webhook",
				metav1.DeleteOptions{},
			)
		}()
		if _, err := e.Audit.WaitFor(ctx, harness.AuditMatch{
			Resource: "kwatch-e2e-missing-webhook",
			Reason:   "WebhookBackendNotFound", Count: 1,
		}); err != nil {
			t.Fatal(err)
		}
	})
}

func TestScenarioExtendedAdmissionPolicy(t *testing.T) {
	runExtendedScenario(t, "security.admission-policy", func(
		ctx context.Context,
		t *testing.T,
		e *harness.Environment,
	) {
		resource := e.Dynamic.Resource(schema.GroupVersionResource{
			Group: "admissionregistration.k8s.io", Version: "v1",
			Resource: "validatingadmissionpolicies",
		})
		name := "kwatch-e2e-invalid-policy"
		object := &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "admissionregistration.k8s.io/v1",
			"kind":       "ValidatingAdmissionPolicy",
			"metadata":   map[string]any{"name": name},
			"spec": map[string]any{
				"failurePolicy": "Ignore",
				"matchConstraints": map[string]any{
					"resourceRules": []any{map[string]any{
						"apiGroups": []any{""}, "apiVersions": []any{"v1"},
						"operations": []any{"CREATE"}, "resources": []any{"pods"},
						"scope": "Namespaced",
					}},
				},
				"validations": []any{map[string]any{
					"expression": "true", "message": "valid",
				}},
			},
		}}
		if _, err := resource.Create(
			ctx, object, metav1.CreateOptions{},
		); err != nil {
			t.Fatal(err)
		}
		defer func() {
			_ = resource.Delete(context.Background(), name,
				metav1.DeleteOptions{})
		}()
		patch := []byte(
			`{"status":{"typeChecking":{"expressionWarnings":[` +
				`{"fieldRef":"spec.validations[0].expression",` +
				`"warning":"e2e"}]}}}`,
		)
		if _, err := resource.Patch(ctx, name, types.MergePatchType,
			patch, metav1.PatchOptions{}, "status"); err != nil {
			t.Fatal(err)
		}
		if _, err := e.Audit.WaitFor(ctx, harness.AuditMatch{
			Resource: name, Reason: "AdmissionPolicyInvalid", Count: 1,
		}); err != nil {
			t.Fatal(err)
		}
	})
}

func TestScenarioExtendedMetricsAPIFailure(t *testing.T) {
	runExtendedScenario(t, "integration.metrics-api", func(
		ctx context.Context,
		t *testing.T,
		e *harness.Environment,
	) {
		namespace := uniqueNamespace(t.Name())
		if err := createNamespace(ctx, e, namespace); err != nil {
			t.Fatal(err)
		}
		defer cleanupNamespace(t, e, namespace)
		apiServices := e.Dynamic.Resource(schema.GroupVersionResource{
			Group: "apiregistration.k8s.io", Version: "v1",
			Resource: "apiservices",
		})
		apiService, err := apiServices.Get(ctx,
			"v1beta1.metrics.k8s.io", metav1.GetOptions{})
		if err != nil {
			t.Fatalf("metrics APIService is required: %v", err)
		}
		service, found, err := unstructured.NestedMap(
			apiService.Object, "spec", "service",
		)
		if err != nil || !found {
			t.Fatal("metrics APIService has no backing Service")
		}
		original := apiService.DeepCopy()
		defer restoreMetricsAPIService(apiServices, original)
		service["name"] = "kwatch-e2e-missing-metrics"
		service["namespace"] = namespace
		if err := unstructured.SetNestedMap(
			apiService.Object, service, "spec", "service",
		); err != nil {
			t.Fatal(err)
		}
		if _, err := apiServices.Update(
			ctx, apiService, metav1.UpdateOptions{},
		); err != nil {
			t.Fatal(err)
		}
		createMetricsHPA(ctx, t, e, namespace)
		if _, err := e.Audit.WaitFor(ctx, harness.AuditMatch{
			Namespace: namespace, Resource: "metrics-target",
			Reason: "FailedGetMetrics", Count: 1,
		}); err != nil {
			t.Fatal(err)
		}
	})
}

func TestScenarioExtendedVolumeAttachment(t *testing.T) {
	runExtendedScenario(t, "storage.volume-attachment", func(
		ctx context.Context,
		t *testing.T,
		e *harness.Environment,
	) {
		nodes, err := e.Client.CoreV1().Nodes().List(ctx,
			metav1.ListOptions{})
		if err != nil || len(nodes.Items) == 0 {
			t.Fatalf("storage scenario requires a schedulable node: %v", err)
		}
		name := uniqueNamespace(t.Name())
		resource := e.Dynamic.Resource(schema.GroupVersionResource{
			Group: "storage.k8s.io", Version: "v1", Resource: "volumeattachments",
		})
		object := &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "storage.k8s.io/v1", "kind": "VolumeAttachment",
			"metadata": map[string]any{"name": name},
			"spec": map[string]any{
				"attacher": "kwatch-e2e.csi", "nodeName": nodes.Items[0].Name,
				"source": map[string]any{"persistentVolumeName": "kwatch-e2e-pv"},
			},
		}}
		if _, err := resource.Create(
			ctx, object, metav1.CreateOptions{},
		); err != nil {
			t.Fatal(err)
		}
		defer func() {
			_ = resource.Delete(context.Background(), name,
				metav1.DeleteOptions{})
		}()
		patch := []byte(fmt.Sprintf(
			`{"status":{"attachError":{"message":"e2e attach failure","time":"%s"}}}`,
			time.Now().UTC().Format(time.RFC3339),
		))
		if _, err := resource.Patch(ctx, name, types.MergePatchType,
			patch, metav1.PatchOptions{}, "status"); err != nil {
			t.Fatal(err)
		}
		if _, err := e.Audit.WaitFor(ctx, harness.AuditMatch{
			Resource: name, Reason: "VolumeAttachmentFailure", Count: 1,
		}); err != nil {
			t.Fatal(err)
		}
	})
}

func expiredCertificate(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), NotBefore: now.Add(-2 * time.Hour),
		NotAfter: now.Add(-time.Hour), BasicConstraintsValid: true,
		DNSNames: []string{"kwatch-e2e.invalid"},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template,
		&key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func createMetricsHPA(
	ctx context.Context,
	t *testing.T,
	e *harness.Environment,
	namespace string,
) {
	t.Helper()
	replicas := int32(1)
	_, err := e.Client.AppsV1().Deployments(namespace).Create(ctx,
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "metrics-target"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{
					"app": "metrics-target",
				}},
				Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "metrics-target"},
				}, Spec: corev1.PodSpec{Containers: []corev1.Container{{
					Name: "workload", Image: workloadImage(),
					Command:         []string{"/kwatch-e2e-workload", "healthy"},
					ImagePullPolicy: corev1.PullIfNotPresent,
					Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
						corev1.ResourceCPU: resourceMustParse("10m"),
					}},
				}}}},
			},
		}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.Client.AutoscalingV2().HorizontalPodAutoscalers(namespace).
		Create(ctx, &autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Name: "metrics-target"},
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
				ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
					APIVersion: "apps/v1", Kind: "Deployment", Name: "metrics-target",
				}, MinReplicas: &replicas, MaxReplicas: 2,
				Metrics: []autoscalingv2.MetricSpec{{
					Type: autoscalingv2.ResourceMetricSourceType,
					Resource: &autoscalingv2.ResourceMetricSource{
						Name: corev1.ResourceCPU,
						Target: autoscalingv2.MetricTarget{
							Type:               autoscalingv2.UtilizationMetricType,
							AverageUtilization: int32Ptr(50)},
					}},
				},
			},
		}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
}

func resourceMustParse(value string) resource.Quantity {
	return resource.MustParse(value)
}

func restoreMetricsAPIService(
	resource dynamic.ResourceInterface,
	original *unstructured.Unstructured,
) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	current, err := resource.Get(ctx, original.GetName(), metav1.GetOptions{})
	if err != nil {
		return
	}
	spec, found, err := unstructured.NestedFieldCopy(
		original.Object, "spec",
	)
	if err != nil || !found {
		return
	}
	if err := unstructured.SetNestedField(
		current.Object, spec, "spec",
	); err != nil {
		return
	}
	_, _ = resource.Update(ctx, current, metav1.UpdateOptions{})
}
