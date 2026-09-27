package network

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	discoveryv1lister "k8s.io/client-go/listers/discovery/v1"
	networkingv1lister "k8s.io/client-go/listers/networking/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/config"
)

func TestServiceExistsSkipsWhenListerIsUnavailable(t *testing.T) {
	runtime := NewRuntimeWithRuntimeConfig(
		config.RuntimeConfig{}, nil, time.Now,
	)

	exists, err := runtime.serviceExists("default", "api")
	if err != nil {
		t.Fatalf("serviceExists() returned error: %v", err)
	}
	if exists {
		t.Fatal("serviceExists() reported an unavailable capability")
	}
}

func TestIngressSkipsWhenServiceListerIsUnavailable(t *testing.T) {
	indexer := cache.NewIndexer(
		cache.MetaNamespaceKeyFunc,
		cache.Indexers{},
	)
	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "frontend",
		},
	}
	if err := indexer.Add(ingress); err != nil {
		t.Fatal(err)
	}
	runtime := NewRuntimeWithRuntimeConfig(
		config.RuntimeConfig{}, &networkSinkRecorder{}, time.Now,
	)
	runtime.ConfigureSources(Sources{
		Ingresses: networkingv1lister.NewIngressLister(indexer),
	})

	if err := runtime.ProcessIngress("default/frontend", false); err != nil {
		t.Fatalf("ProcessIngress() returned error: %v", err)
	}
}

func TestServiceSkipsWhenBackendPodSourceIsUnavailable(t *testing.T) {
	indexer := cache.NewIndexer(
		cache.MetaNamespaceKeyFunc,
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
	)
	sink := &networkSinkRecorder{}
	runtime := NewRuntimeWithRuntimeConfig(
		config.RuntimeConfig{}, sink, time.Now,
	)
	if err := runtime.ConfigureSources(Sources{
		EndpointSlice: discoveryv1lister.NewEndpointSliceLister(indexer),
	}); err != nil {
		t.Fatal(err)
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "api"},
	}
	if err := runtime.ProcessServiceObject(svc, true); err != nil {
		t.Fatal(err)
	}
	if sink.gone != 0 {
		t.Fatal("missing pod source resolved Service incidents")
	}
}
