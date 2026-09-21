package crdwatch

import (
	"context"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/abahmed/kwatch/internal/config"
)

func TestWatcherStartsInformerAndStops(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(), map[schema.GroupVersionResource]string{
			gvr: "KwatchConfigList",
		})
	cfg := &config.Config{}
	cfg.CrdConfig.Enabled = true
	watcher := NewWithClient(
		config.RuntimeConfigFor(cfg), client, "default", 0,
		func() {}, nil, nil,
	)
	if err := watcher.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if watcher.Status().State == "stopped" {
		t.Fatal("started watcher reported stopped")
	}
	if err := watcher.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if watcher.Status().State != "stopped" {
		t.Fatal("stopped watcher did not report stopped")
	}
}

func TestWatcherConfigurationValidationHelpers(t *testing.T) {
	spec := map[string]interface{}{
		"pendingPodThreshold": int64(3),
	}
	normalizeLegacySpec(spec)
	monitor, ok := spec["pendingPodMonitor"].(map[string]interface{})
	if !ok || monitor["threshold"] != int64(3) {
		t.Fatalf("legacy spec normalization = %#v", spec)
	}
	normalizeLegacySpec(map[string]interface{}{
		"pendingPodThreshold": int64(4),
		"pendingPodMonitor":   map[string]interface{}{"threshold": int64(5)},
	})
	if err := rejectSecretConfig(map[string]interface{}{
		"alert": map[string]interface{}{},
	}); err == nil {
		t.Fatal("alert credentials were accepted")
	}
	if err := rejectSecretConfig(map[string]interface{}{
		"healthCheck": map[string]interface{}{"diagnosticsToken": "x"},
	}); err == nil {
		t.Fatal("diagnostic token was accepted")
	}
	if err := rejectSecretConfig(map[string]interface{}{
		"heartbeatMonitor": map[string]interface{}{"url": "x"},
	}); err == nil {
		t.Fatal("heartbeat URL was accepted")
	}
	if err := rejectSecretConfig("not a map"); err != nil {
		t.Fatal(err)
	}
	if !isMissingCRD(apierrors.NewNotFound(schema.GroupResource{
		Resource: "kwatchconfigs",
	}, "one")) {
		t.Fatal("NotFound was not recognized as missing CRD")
	}
	object := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "one"},
	}}
	if object.GetName() != "one" {
		t.Fatal("unstructured fixture was malformed")
	}
}
