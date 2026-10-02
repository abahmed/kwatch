package app

import (
	"context"
	"errors"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/health"
)

var kwatchConfigResource = schema.GroupVersionResource{
	Group: "kwatch.abahmed.dev", Version: "v1alpha1",
	Resource: "kwatchconfigs",
}

func overlayClient(
	spec map[string]interface{},
) *dynamicfake.FakeDynamicClient {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "kwatch.abahmed.dev/v1alpha1",
		"kind":       "KwatchConfig",
		"metadata": map[string]interface{}{
			"name": "kwatch", "namespace": "kwatch",
		},
		"spec": spec,
	}}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			kwatchConfigResource: "KwatchConfigList",
		},
		obj,
	)
}

func crdBase() *config.Config {
	cfg := config.DefaultConfig()
	cfg.CrdConfig.Enabled = true
	cfg.App.ClusterName = "base"
	return cfg
}

func TestApplyStartupCRDFallsBackToBaseOnInvalidOverlay(t *testing.T) {
	t.Setenv("POD_NAMESPACE", "kwatch")
	client := overlayClient(map[string]interface{}{
		"app":        map[string]interface{}{"clusterName": "overlay"},
		"namespaces": []interface{}{"apps", "!kube-system"},
	})
	reloads := 0
	reload := func() (*config.Config, error) {
		reloads++
		return crdBase(), nil
	}

	cfg, invalid, err := applyStartupCRD(
		context.Background(), crdBase(), client, reload)

	if err != nil || !invalid {
		t.Fatalf("invalid = %v, err = %v; want fallback", invalid, err)
	}
	if reloads != 1 || cfg.App.ClusterName != "base" {
		t.Fatalf("reloads = %d, cluster = %q; want the fresh base",
			reloads, cfg.App.ClusterName)
	}
}

func TestApplyStartupCRDAppliesValidOverlay(t *testing.T) {
	t.Setenv("POD_NAMESPACE", "kwatch")
	client := overlayClient(map[string]interface{}{
		"app": map[string]interface{}{"clusterName": "overlay"},
	})
	reload := func() (*config.Config, error) {
		t.Fatal("a valid overlay must not reload the base")
		return nil, nil
	}

	cfg, invalid, err := applyStartupCRD(
		context.Background(), crdBase(), client, reload)

	if err != nil || invalid || cfg.App.ClusterName != "overlay" {
		t.Fatalf("cfg = %v, invalid = %v, err = %v", cfg, invalid, err)
	}
}

func TestApplyStartupCRDFailsOnAPIError(t *testing.T) {
	client := overlayClient(nil)
	client.PrependReactor("list", "kwatchconfigs",
		func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("api unavailable")
		})

	_, invalid, err := applyStartupCRD(
		context.Background(), crdBase(), client, loadConfig)

	if err == nil || invalid {
		t.Fatalf("invalid = %v, err = %v; want a startup error",
			invalid, err)
	}
}

func TestReportOverlayHealthPublishesBoundedReason(t *testing.T) {
	server := health.NewHealthServerWithClock(
		config.HealthCheck{}, clock.RealClock{})

	reportOverlayHealth(server, false)
	if _, ok := server.ComponentStatuses()[overlayHealthComponent]; ok {
		t.Fatal("a valid overlay must not publish a status")
	}

	reportOverlayHealth(server, true)
	status := server.ComponentStatuses()[overlayHealthComponent]
	if status.State != "degraded" ||
		status.Reason != "config_overlay_invalid" {
		t.Fatalf("status = %+v", status)
	}
}
