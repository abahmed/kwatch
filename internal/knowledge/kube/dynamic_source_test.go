package kube_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
)

func crd(group, kind, plural string, status bool) *unstructured.Unstructured {
	version := map[string]any{
		"name": "v1", "served": true, "storage": true,
	}
	if status {
		version["subresources"] = map[string]any{
			"status": map[string]any{},
		}
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apiextensions.k8s.io/v1",
		"kind":       "CustomResourceDefinition",
		"metadata":   map[string]any{"name": plural + "." + group},
		"spec": map[string]any{
			"group":    group,
			"names":    map[string]any{"kind": kind, "plural": plural},
			"versions": []any{version},
		},
	}}
}

func TestDynamicSourceWatchesStatusResources(t *testing.T) {
	apiService := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apiregistration.k8s.io/v1", "kind": "APIService",
		"metadata": map[string]any{"name": "v1.metrics.example"},
		"spec": map[string]any{"service": map[string]any{
			"namespace": "sys", "name": "metrics",
		}},
	}}
	widget := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "example.com/v1", "kind": "Widget",
		"metadata": map[string]any{"name": "w", "namespace": "ns"},
	}}
	kinds := map[schema.GroupVersionResource]string{
		{Group: "apiextensions.k8s.io", Version: "v1",
			Resource: "customresourcedefinitions"}: "CustomResourceDefinitionList",
		{Group: "apiregistration.k8s.io", Version: "v1",
			Resource: "apiservices"}: "APIServiceList",
		{Group: "example.com", Version: "v1",
			Resource: "widgets"}: "WidgetList",
	}
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(), kinds,
		crd("example.com", "Widget", "widgets", true),
		crd("example.com", "Plain", "plains", false),
		apiService, widget)
	facts := make(chan knowledge.Fact, 16)
	src := kube.NewDynamicSource(kube.DynamicConfig{
		Client: client, Now: fixedTime, Max: 5,
		Submit: func(_ context.Context, f ...knowledge.Fact) {
			for _, fact := range f {
				facts <- fact
			}
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { src.Run(ctx); close(done) }()

	seen := map[knowledge.Kind]bool{}
	for !(seen["apiservice"] && seen["widget"]) {
		fact := <-facts
		seen[fact.Entity.Kind] = true
	}
	cancel()
	<-done
	assert.False(t, seen["plain"])
}

func TestDynamicSourceCapsCustomResourceTypes(t *testing.T) {
	kinds := map[schema.GroupVersionResource]string{
		{Group: "apiextensions.k8s.io", Version: "v1",
			Resource: "customresourcedefinitions"}: "CustomResourceDefinitionList",
		{Group: "apiregistration.k8s.io", Version: "v1",
			Resource: "apiservices"}: "APIServiceList",
		{Group: "example.com", Version: "v1",
			Resource: "as"}: "AList",
		{Group: "example.com", Version: "v1",
			Resource: "bs"}: "BList",
	}
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(), kinds,
		crd("example.com", "A", "as", true),
		crd("example.com", "B", "bs", true))
	src := kube.NewDynamicSource(kube.DynamicConfig{
		Client: client, Now: fixedTime, Max: 1,
		Submit: func(context.Context, ...knowledge.Fact) {},
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { src.Run(ctx); close(done) }()
	cancel()
	<-done
}
