package crd

import (
	"context"
	"errors"
	"os"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"sigs.k8s.io/yaml"

	"github.com/abahmed/kwatch/internal/config"
)

func freshBase() (*config.Config, error) {
	return config.DefaultConfig(), nil
}

func kwatchConfig(
	version string, spec map[string]interface{},
) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "kwatch.abahmed.dev/v1alpha1",
		"kind":       "KwatchConfig",
	}}
	if spec != nil {
		obj.Object["spec"] = spec
	}
	obj.SetNamespace("kwatch")
	obj.SetName("kwatch")
	obj.SetResourceVersion(version)
	return obj
}

func mixedNamespaces() map[string]interface{} {
	return map[string]interface{}{
		"namespaces": []interface{}{"apps", "!kube-system"},
	}
}

func TestWatcherRejectsInvalidKwatchConfigEdit(t *testing.T) {
	restarts := 0
	var states []Status
	watcher := &Watcher{
		restart:   func() { restarts++ },
		seen:      map[string]string{"kwatch/kwatch": "1"},
		ready:     true,
		started:   true,
		loadBase:  freshBase,
		stateSink: func(status Status) { states = append(states, status) },
	}

	watcher.changed(kwatchConfig("2", mixedNamespaces()))

	if restarts != 0 {
		t.Fatalf("an invalid edit must not restart kwatch, got %d", restarts)
	}
	status := watcher.Status()
	if status.State != "degraded" ||
		status.LastError != overlayInvalidReason {
		t.Fatalf("status = %+v, want config_overlay_invalid", status)
	}
	if len(states) != 1 {
		t.Fatalf("state sink calls = %d, want 1", len(states))
	}

	watcher.changed(kwatchConfig("2", mixedNamespaces()))
	if len(states) != 1 {
		t.Fatal("a resync of the rejected version must not be re-reported")
	}

	watcher.changed(kwatchConfig("3", map[string]interface{}{
		"namespaces": []interface{}{"apps"},
	}))
	if restarts != 1 {
		t.Fatalf("a fixed KwatchConfig must restart, got %d", restarts)
	}
}

func TestWatcherRejectsForbiddenFieldEdit(t *testing.T) {
	watcher := &Watcher{
		restart:  func() { t.Fatal("a forbidden field must not restart") },
		seen:     map[string]string{"kwatch/kwatch": "1"},
		ready:    true,
		loadBase: freshBase,
	}
	watcher.changed(kwatchConfig("2", map[string]interface{}{
		"app": map[string]interface{}{"proxyURL": "http://proxy"},
	}))
}

func TestWatcherRejectsSecondKwatchConfig(t *testing.T) {
	watcher := &Watcher{
		restart:  func() { t.Fatal("two KwatchConfigs must not restart") },
		seen:     map[string]string{"kwatch/other": "1"},
		ready:    true,
		loadBase: freshBase,
	}
	watcher.changed(kwatchConfig("1", nil))
}

func TestWatcherRejectsInvalidLateKwatchConfig(t *testing.T) {
	watcher := &Watcher{
		restart:  func() { t.Fatal("an invalid late config must not restart") },
		seen:     make(map[string]string),
		loadBase: freshBase,
	}
	if watcher.restartForLateObjects([]interface{}{
		kwatchConfig("1", mixedNamespaces()),
	}) {
		t.Fatal("restart reported for an invalid late config")
	}
	if watcher.seen["kwatch/kwatch"] != "1" {
		t.Fatalf("rejected object was not remembered: %v", watcher.seen)
	}
}

func TestApplyOverlayMarksEveryFailureInvalid(t *testing.T) {
	for name, spec := range map[string]map[string]interface{}{
		"validation": mixedNamespaces(),
		"forbidden": {
			"auditLog": map[string]interface{}{"output": "/tmp/x"},
		},
		"credentials": {"alert": map[string]interface{}{}},
	} {
		t.Run(name, func(t *testing.T) {
			err := ApplyOverlay(config.DefaultConfig(),
				kwatchConfig("1", spec))
			if !errors.Is(err, ErrInvalidOverlay) {
				t.Fatalf("err = %v, want ErrInvalidOverlay", err)
			}
		})
	}
}

func TestStartupOverlayRejectsSecondObjectAsInvalid(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{gvr: "KwatchConfigList"},
	)
	for _, name := range []string{"one", "two"} {
		obj := kwatchConfig("", nil)
		obj.SetName(name)
		if _, err := client.Resource(gvr).Namespace("kwatch").Create(
			context.Background(), obj, metav1.CreateOptions{},
		); err != nil {
			t.Fatal(err)
		}
	}
	cfg := config.DefaultConfig()
	cfg.CrdConfig.Enabled = true

	err := applyStartupConfig(context.Background(), cfg, client, "kwatch")
	if !errors.Is(err, ErrInvalidOverlay) {
		t.Fatalf("err = %v, want ErrInvalidOverlay", err)
	}
}

func TestOverlayCannotReenableSecretWatching(t *testing.T) {
	t.Setenv("KWATCH_WATCH_SECRETS", "false")
	cfg := config.DefaultConfig()

	err := ApplyOverlay(cfg, kwatchConfig("1", map[string]interface{}{
		"watch": map[string]interface{}{"secrets": true},
	}))

	if err != nil {
		t.Fatal(err)
	}
	if cfg.Watch.Secrets || cfg.Runtime.Application().WatchSecrets {
		t.Fatal("KWATCH_WATCH_SECRETS=false must win over the overlay")
	}
}

// TestCRDSchemaOmitsForbiddenFields keeps the published schema in step with
// the runtime: a field the overlay rejects must not be offered by the CRD.
func TestCRDSchemaOmitsForbiddenFields(t *testing.T) {
	raw, err := os.ReadFile("../../../deploy/crd.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var crd struct {
		Spec struct {
			Versions []struct {
				Schema struct {
					OpenAPIV3Schema schemaNode `json:"openAPIV3Schema"`
				} `json:"schema"`
			} `json:"versions"`
		} `json:"spec"`
	}
	if err := yaml.Unmarshal(raw, &crd); err != nil {
		t.Fatal(err)
	}
	for _, version := range crd.Spec.Versions {
		spec := version.Schema.OpenAPIV3Schema.Properties["spec"]
		if _, ok := spec.Properties["alert"]; ok {
			t.Error("CRD schema offers spec.alert")
		}
		for _, path := range forbiddenSpecPaths {
			section := spec.Properties[path[0]]
			if _, ok := section.Properties[path[1]]; ok {
				t.Errorf("CRD schema offers forbidden spec.%s.%s",
					path[0], path[1])
			}
		}
	}
}

type schemaNode struct {
	Properties map[string]schemaNode `json:"properties"`
}
