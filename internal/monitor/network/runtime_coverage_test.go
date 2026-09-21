package network

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
	discoveryv1lister "k8s.io/client-go/listers/discovery/v1"
	networkingv1lister "k8s.io/client-go/listers/networking/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/config"
)

func TestNetworkRuntimeHandlesInvalidKeysAndDeletion(t *testing.T) {
	runtime := NewRuntimeWithRuntimeConfig(
		config.RuntimeConfig{}, &networkSinkRecorder{}, time.Now,
	)
	for _, call := range []func(string) error{
		func(key string) error { return runtime.ProcessService(key, false) },
		func(key string) error {
			return runtime.ProcessNetworkPolicy(key, false)
		},
		func(key string) error { return runtime.ProcessIngress(key, false) },
	} {
		if err := call("too/many/parts"); err == nil {
			t.Fatal("invalid queue key was accepted")
		}
	}
	if err := runtime.ProcessServiceObject(nil, false); err != nil {
		t.Fatal(err)
	}
}

func TestNetworkRuntimeProcessesServiceAndIngressFromListers(t *testing.T) {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "api"},
	}
	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "api"},
	}
	services := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	ingresses := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	endpoints := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	if err := services.Add(service); err != nil {
		t.Fatal(err)
	}
	if err := ingresses.Add(ingress); err != nil {
		t.Fatal(err)
	}
	sink := &networkSinkRecorder{}
	runtime := NewRuntimeWithRuntimeConfig(
		config.RuntimeConfig{}, sink, time.Now,
	)
	if err := runtime.ConfigureSources(Sources{
		Services:      corev1lister.NewServiceLister(services),
		Ingresses:     networkingv1lister.NewIngressLister(ingresses),
		EndpointSlice: discoveryv1lister.NewEndpointSliceLister(endpoints),
	}); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ProcessService("apps/api", false); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ProcessService("apps/api", true); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ProcessIngress("apps/api", false); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ProcessIngress("apps/api", true); err != nil {
		t.Fatal(err)
	}
	if sink.gone != 2 {
		t.Fatalf("gone reconciliations = %d, want 2", sink.gone)
	}
}
