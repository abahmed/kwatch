package security

import (
	"testing"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	admv1lister "k8s.io/client-go/listers/admissionregistration/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/config"
)

func TestServiceExistsSkipsWhenListerIsUnavailable(t *testing.T) {
	runtime := NewRuntimeWithRuntimeConfig(
		config.RuntimeConfig{}, nil,
	)

	exists, err := runtime.serviceExists("default", "api")
	if err != nil {
		t.Fatalf("serviceExists() returned error: %v", err)
	}
	if exists {
		t.Fatal("serviceExists() reported an unavailable capability")
	}
}

func TestWebhookSkipsUnavailableDependencyListers(t *testing.T) {
	indexer := cache.NewIndexer(
		cache.MetaNamespaceKeyFunc,
		cache.Indexers{},
	)
	configuration := &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "webhook"},
	}
	if err := indexer.Add(configuration); err != nil {
		t.Fatal(err)
	}
	sink := &tlsSink{}
	runtime := NewRuntimeWithRuntimeConfig(
		config.RuntimeConfig{}, sink,
	)
	runtime.SetSources(Sources{
		MutatingWebhooks: admv1lister.NewMutatingWebhookConfigurationLister(indexer),
	})

	if err := runtime.ProcessMutatingWebhookConfiguration(
		"webhook", false,
	); err != nil {
		t.Fatalf(
			"ProcessMutatingWebhookConfiguration() returned error: %v",
			err,
		)
	}
	if len(sink.observations) != 0 {
		t.Fatalf(
			"got %d observations with unavailable dependency listers",
			len(sink.observations),
		)
	}
}
