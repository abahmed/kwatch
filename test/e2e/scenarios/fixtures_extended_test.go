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
	"os"
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
)

const (
	extWebhookName      = "kwatch-e2e-missing-webhook"
	extMetricsAPIName   = "v1beta1.metrics.k8s.io"
	extCleanupTimeout   = time.Minute
	extMetricsTargetApp = "metrics-target"
)

// inExtendedNamespace is inNamespace for the extended scenarios, which
// only run when KWATCH_EXTENDED=true.
func inExtendedNamespace(t *testing.T, id string, run func(s *Scenario)) {
	t.Helper()
	skipUnlessExtended(t)
	inNamespace(t, id, run)
}

// inExtendedCluster is onCluster for the extended scenarios.
func inExtendedCluster(t *testing.T, id string, run func(s *Scenario)) {
	t.Helper()
	skipUnlessExtended(t)
	onCluster(t, id, run)
}

func skipUnlessExtended(t *testing.T) {
	t.Helper()
	if os.Getenv("KWATCH_EXTENDED") != "true" {
		t.Skip("set KWATCH_EXTENDED=true for extended Kind scenarios")
	}
}

// extCreateExpiredTLSSecret creates a TLS Secret whose certificate ended
// an hour ago.
func extCreateExpiredTLSSecret(s *Scenario, name string) {
	s.T.Helper()
	_, err := s.Env.Client.CoreV1().Secrets(s.Namespace).Create(s.Ctx,
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Type:       corev1.SecretTypeTLS,
			Data: map[string][]byte{
				"tls.crt": extExpiredCertificate(s.T),
				"tls.key": []byte("not-used-by-monitor"),
			},
		}, metav1.CreateOptions{})
	s.Must(err)
}

func extExpiredCertificate(t *testing.T) []byte {
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

// extCreateWebhookWithoutService registers a validating webhook whose
// backend Service does not exist and removes it when the test ends.
func extCreateWebhookWithoutService(s *Scenario) {
	s.T.Helper()
	failurePolicy := admissionregistrationv1.Ignore
	sideEffects := admissionregistrationv1.SideEffectClassNone
	hooks := s.Env.Client.AdmissionregistrationV1().
		ValidatingWebhookConfigurations()
	_, err := hooks.Create(s.Ctx,
		&admissionregistrationv1.ValidatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{Name: extWebhookName},
			Webhooks: []admissionregistrationv1.ValidatingWebhook{{
				Name: "missing.kwatch.e2e",
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					Service: &admissionregistrationv1.ServiceReference{
						Name:      "missing-webhook",
						Namespace: s.Namespace,
					}},
				AdmissionReviewVersions: []string{"v1"},
				FailurePolicy:           &failurePolicy,
				SideEffects:             &sideEffects,
			}},
		}, metav1.CreateOptions{})
	s.Must(err)
	s.T.Cleanup(func() {
		ctx, cancel := context.WithTimeout(
			context.Background(), extCleanupTimeout)
		defer cancel()
		_ = hooks.Delete(ctx, extWebhookName, metav1.DeleteOptions{})
	})
}

// extBreakMetricsAPI points the metrics APIService at a Service that does
// not exist and restores the original spec when the test ends.
func extBreakMetricsAPI(s *Scenario) {
	s.T.Helper()
	apiServices := s.Env.Dynamic.Resource(schema.GroupVersionResource{
		Group: "apiregistration.k8s.io", Version: "v1",
		Resource: "apiservices",
	})
	apiService, err := apiServices.Get(s.Ctx, extMetricsAPIName,
		metav1.GetOptions{})
	if err != nil {
		s.T.Fatalf("metrics APIService is required: %v", err)
	}
	service, found, err := unstructured.NestedMap(
		apiService.Object, "spec", "service")
	if err != nil || !found {
		s.T.Fatal("metrics APIService has no backing Service")
	}
	original := apiService.DeepCopy()
	s.T.Cleanup(func() { extRestoreMetricsAPI(s, original) })
	service["name"] = "kwatch-e2e-missing-metrics"
	service["namespace"] = s.Namespace
	s.Must(unstructured.SetNestedMap(
		apiService.Object, service, "spec", "service"))
	_, err = apiServices.Update(s.Ctx, apiService, metav1.UpdateOptions{})
	s.Must(err)
}

func extRestoreMetricsAPI(s *Scenario, original *unstructured.Unstructured) {
	ctx, cancel := context.WithTimeout(
		context.Background(), extCleanupTimeout)
	defer cancel()
	apiServices := s.Env.Dynamic.Resource(schema.GroupVersionResource{
		Group: "apiregistration.k8s.io", Version: "v1",
		Resource: "apiservices",
	})
	current, err := apiServices.Get(ctx, original.GetName(),
		metav1.GetOptions{})
	if err != nil {
		return
	}
	spec, found, err := unstructured.NestedFieldCopy(original.Object, "spec")
	if err != nil || !found {
		return
	}
	if unstructured.SetNestedField(current.Object, spec, "spec") != nil {
		return
	}
	_, _ = apiServices.Update(ctx, current, metav1.UpdateOptions{})
}

// extCreateAutoscaledDeployment creates a Deployment and an HPA that scales
// it on CPU, so the autoscaler asks the metrics API for numbers.
func extCreateAutoscaledDeployment(s *Scenario) {
	s.T.Helper()
	labels := map[string]string{"app": extMetricsTargetApp}
	replicas := int32(1)
	_, err := s.Env.Client.AppsV1().Deployments(s.Namespace).Create(s.Ctx,
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: extMetricsTargetApp},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: labels},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: labels},
					Spec: corev1.PodSpec{Containers: []corev1.Container{{
						Name: "workload", Image: workloadImage(),
						Command: []string{
							"/kwatch-e2e-workload", "healthy"},
						ImagePullPolicy: corev1.PullIfNotPresent,
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU: resource.MustParse("10m"),
							}},
					}}},
				},
			},
		}, metav1.CreateOptions{})
	s.Must(err)
	_, err = s.Env.Client.AutoscalingV2().
		HorizontalPodAutoscalers(s.Namespace).Create(s.Ctx,
		extCPUAutoscaler(replicas), metav1.CreateOptions{})
	s.Must(err)
}

func extCPUAutoscaler(
	replicas int32,
) *autoscalingv2.HorizontalPodAutoscaler {
	return &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: extMetricsTargetApp},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1", Kind: "Deployment",
				Name: extMetricsTargetApp,
			},
			MinReplicas: &replicas, MaxReplicas: 2,
			Metrics: []autoscalingv2.MetricSpec{{
				Type: autoscalingv2.ResourceMetricSourceType,
				Resource: &autoscalingv2.ResourceMetricSource{
					Name: corev1.ResourceCPU,
					Target: autoscalingv2.MetricTarget{
						Type:               autoscalingv2.UtilizationMetricType,
						AverageUtilization: int32Ptr(50),
					},
				},
			}},
		},
	}
}

// extCreateFailedVolumeAttachment creates a VolumeAttachment, reports an
// attach error on its status and returns its name.
func extCreateFailedVolumeAttachment(s *Scenario) string {
	s.T.Helper()
	nodes, err := s.Env.Client.CoreV1().Nodes().List(s.Ctx,
		metav1.ListOptions{})
	if err != nil || len(nodes.Items) == 0 {
		s.T.Fatalf("storage scenario requires a schedulable node: %v", err)
	}
	name := uniqueNamespace(s.T.Name())
	attachments := s.Env.Dynamic.Resource(schema.GroupVersionResource{
		Group: "storage.k8s.io", Version: "v1",
		Resource: "volumeattachments",
	})
	_, err = attachments.Create(s.Ctx, &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "storage.k8s.io/v1", "kind": "VolumeAttachment",
			"metadata": map[string]any{"name": name},
			"spec": map[string]any{
				"attacher": "kwatch-e2e.csi",
				"nodeName": nodes.Items[0].Name,
				"source": map[string]any{
					"persistentVolumeName": "kwatch-e2e-pv"},
			},
		}}, metav1.CreateOptions{})
	s.Must(err)
	s.T.Cleanup(func() {
		ctx, cancel := context.WithTimeout(
			context.Background(), extCleanupTimeout)
		defer cancel()
		_ = attachments.Delete(ctx, name, metav1.DeleteOptions{})
	})
	patch := []byte(fmt.Sprintf(
		`{"status":{"attachError":{"message":"e2e attach failure",`+
			`"time":"%s"}}}`, time.Now().UTC().Format(time.RFC3339)))
	_, err = attachments.Patch(s.Ctx, name, types.MergePatchType, patch,
		metav1.PatchOptions{}, "status")
	s.Must(err)
	return name
}
