package crdwatch

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/abahmed/kwatch/internal/config"
)

func TestApplyStartupConfigHandlesEmptyAndInvalidLists(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{gvr: "KwatchConfigList"},
	)
	cfg := config.DefaultConfig()
	cfg.CrdConfig.Enabled = true
	if err := applyStartupConfig(
		context.Background(), cfg, client, "default",
	); err != nil {
		t.Fatalf("empty CRD list = %v", err)
	}
	first := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "kwatch.abahmed.dev/v1alpha1",
		"kind":       "KwatchConfig",
		"metadata": map[string]interface{}{
			"name": "one", "namespace": "default",
		},
	}}
	if _, err := client.Resource(gvr).Namespace("default").Create(
		context.Background(), first, metav1.CreateOptions{},
	); err != nil {
		t.Fatal(err)
	}
	if err := applyStartupConfig(
		context.Background(), cfg, client, "default",
	); err != nil {
		t.Fatalf("object without spec = %v", err)
	}
	second := first.DeepCopy()
	second.SetName("two")
	if _, err := client.Resource(gvr).Namespace("default").Create(
		context.Background(), second, metav1.CreateOptions{},
	); err != nil {
		t.Fatal(err)
	}
	if err := applyStartupConfig(
		context.Background(), cfg, client, "default",
	); err == nil {
		t.Fatal("multiple KwatchConfig objects were accepted")
	}
}

func TestWatcherChangedRequestsRestartOnlyForNewResourceVersion(t *testing.T) {
	restarts := 0
	watcher := &Watcher{
		seen:    make(map[string]string),
		restart: func() { restarts++ },
		ready:   true,
	}
	object := &unstructured.Unstructured{Object: map[string]interface{}{}}
	object.SetNamespace("apps")
	object.SetName("config")
	object.SetResourceVersion("1")
	watcher.changed(object)
	watcher.changed(object)
	object.SetResourceVersion("2")
	watcher.changed(object)
	if restarts != 1 || watcher.seen["apps/config"] != "1" {
		t.Fatalf("restart=%d seen=%v", restarts, watcher.seen)
	}
}
