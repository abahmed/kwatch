package crd

import (
	"context"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/abahmed/kwatch/internal/config"
)

func TestApplyStartupConfigReportsKwatchConfigTypos(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{gvr: "KwatchConfigList"},
	)
	object := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "kwatch.abahmed.dev/v1alpha1",
		"kind":       "KwatchConfig",
		"metadata": map[string]interface{}{
			"name": "one", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"app": map[string]interface{}{
				"clusterName": "prod", "clusterNmae": "typo",
			},
		},
	}}
	if _, err := client.Resource(gvr).Namespace("default").Create(
		context.Background(), object, metav1.CreateOptions{},
	); err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultConfig()
	cfg.CrdConfig.Enabled = true

	if err := applyStartupConfig(
		context.Background(), cfg, client, "default",
	); err != nil {
		t.Fatal(err)
	}

	if cfg.App.ClusterName != "prod" {
		t.Fatalf("known overlay keys must apply: %q", cfg.App.ClusterName)
	}
	var found bool
	for _, warning := range config.Warnings(cfg) {
		if strings.Contains(warning, "KwatchConfig spec") &&
			strings.Contains(warning, "app.clusterNmae") {
			found = true
		}
	}
	if !found {
		t.Fatalf("typo not reported: %v", config.Warnings(cfg))
	}
}
