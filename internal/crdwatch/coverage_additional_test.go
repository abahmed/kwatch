package crdwatch

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"

	"github.com/abahmed/kwatch/internal/config"
)

func TestApplyStartupConfigHandlesDisabledAndMissingClients(t *testing.T) {
	if err := ApplyStartupConfigWithClient(
		context.Background(), &config.Config{}, nil, "kwatch",
	); err != nil {
		t.Fatalf("disabled overlay returned error: %v", err)
	}
	if err := ApplyStartupConfigWithClient(
		context.Background(), &config.Config{
			CrdConfig: config.CrdConfig{Enabled: true},
		}, nil, "kwatch",
	); err == nil {
		t.Fatal("enabled overlay accepted a nil dynamic client")
	}
}

func TestApplyStartupConfigHandlesEmptyAndMultipleObjects(t *testing.T) {
	runtimeScheme := fake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			gvr: "KwatchConfigList",
		},
	)
	cfg := &config.Config{CrdConfig: config.CrdConfig{Enabled: true}}
	if err := ApplyStartupConfigWithClient(
		context.Background(), cfg, runtimeScheme, "kwatch",
	); err != nil {
		t.Fatalf("empty overlay returned error: %v", err)
	}

	objects := []runtime.Object{
		&unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "kwatch.abahmed.dev/v1alpha1",
			"kind":       "KwatchConfig",
			"metadata": map[string]interface{}{
				"name": "one", "namespace": "kwatch",
			},
		}},
		&unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "kwatch.abahmed.dev/v1alpha1",
			"kind":       "KwatchConfig",
			"metadata": map[string]interface{}{
				"name": "two", "namespace": "kwatch",
			},
		}},
	}
	client := fake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			gvr: "KwatchConfigList",
		},
		objects...,
	)
	if err := ApplyStartupConfigWithClient(
		context.Background(), &config.Config{
			CrdConfig: config.CrdConfig{Enabled: true},
		}, client, "kwatch",
	); err == nil {
		t.Fatal("multiple startup objects were accepted")
	}
}
