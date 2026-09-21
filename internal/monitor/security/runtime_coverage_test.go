package security

import (
	"testing"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	admv1lister "k8s.io/client-go/listers/admissionregistration/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
	discoveryv1lister "k8s.io/client-go/listers/discovery/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/config"
)

func TestSecurityRuntimeProcessesBothWebhookKinds(t *testing.T) {
	mutating := &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "mutating"},
	}
	validating := &admissionregistrationv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "validating"},
	}
	mutatingIndex := cache.NewIndexer(
		cache.MetaNamespaceKeyFunc, cache.Indexers{},
	)
	validatingIndex := cache.NewIndexer(
		cache.MetaNamespaceKeyFunc, cache.Indexers{},
	)
	services := cache.NewIndexer(
		cache.MetaNamespaceKeyFunc, cache.Indexers{},
	)
	endpoints := cache.NewIndexer(
		cache.MetaNamespaceKeyFunc, cache.Indexers{},
	)
	for _, object := range []interface{}{mutating} {
		if err := mutatingIndex.Add(object); err != nil {
			t.Fatal(err)
		}
	}
	if err := validatingIndex.Add(validating); err != nil {
		t.Fatal(err)
	}
	if err := services.Add(&corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "apps"},
	}); err != nil {
		t.Fatal(err)
	}
	runtime := NewRuntimeWithRuntimeConfig(config.RuntimeConfig{}, &tlsSink{})
	if err := runtime.ConfigureSources(Sources{
		MutatingWebhooks: admv1lister.NewMutatingWebhookConfigurationLister(
			mutatingIndex,
		),
		ValidatingWebhooks: admv1lister.NewValidatingWebhookConfigurationLister(
			validatingIndex,
		),
		Services:       corev1lister.NewServiceLister(services),
		EndpointSlices: discoveryv1lister.NewEndpointSliceLister(endpoints),
	}); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ProcessMutatingWebhookConfiguration(
		"mutating", false,
	); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ProcessValidatingWebhookConfiguration(
		"validating", false,
	); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ProcessMutatingWebhookConfiguration(
		"mutating", true,
	); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ProcessValidatingWebhookConfiguration(
		"validating", true,
	); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ConfigureSources(Sources{}); err == nil {
		t.Fatal("second security source configuration succeeded")
	}
}
