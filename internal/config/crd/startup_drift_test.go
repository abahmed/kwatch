package crd

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"

	"github.com/abahmed/kwatch/internal/config"
)

func driftObject(spec map[string]interface{}) *unstructured.Unstructured {
	obj := kwatchConfigObject("kwatch")
	if spec != nil {
		obj.Object["spec"] = spec
	}
	return obj
}

func shopOnly() map[string]interface{} {
	return map[string]interface{}{
		"namespaces": []interface{}{"shop"},
	}
}

func driftConfig() *config.Config {
	cfg := config.DefaultConfig()
	cfg.CrdConfig.Enabled = true
	return cfg
}

// bootSignature runs the startup overlay the way bootstrap does and
// returns the signature the application hands to the watcher.
func bootSignature(t *testing.T, client dynamic.Interface) string {
	t.Helper()
	spec, err := ApplyStartupConfigWithClient(
		context.Background(), driftConfig(), client, "default")
	if err != nil && !errors.Is(err, ErrInvalidOverlay) {
		t.Fatalf("startup overlay: %v", err)
	}
	return StartupSignature(spec, err != nil)
}

// startedRestarts starts a watcher that expects signature and reports how
// many restarts its first synced list asked for.
func startedRestarts(
	t *testing.T, client dynamic.Interface, signature string,
) int64 {
	t.Helper()
	var restarts atomic.Int64
	watcher := NewWithClient(
		config.RuntimeConfigFor(driftConfig()), client, "default", 0,
		func() { restarts.Add(1) }, nil, nil, freshBase,
	)
	watcher.ExpectStartup(signature)
	if err := watcher.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = watcher.Stop(context.Background()) })
	return restarts.Load()
}

func kwatchConfigs(client dynamic.Interface) dynamic.ResourceInterface {
	return client.Resource(gvr).Namespace("default")
}

func TestWatcherRestartsForConfigCreatedBeforeItStarted(t *testing.T) {
	client := newFakeKwatchConfigClient()
	signature := bootSignature(t, client)
	_, err := kwatchConfigs(client).Create(context.Background(),
		driftObject(shopOnly()), metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if got := startedRestarts(t, client, signature); got != 1 {
		t.Fatalf("restarts = %d, want 1 for a late KwatchConfig", got)
	}
}

func TestWatcherRestartsForConfigEditedBeforeItStarted(t *testing.T) {
	client := newFakeKwatchConfigClient(driftObject(shopOnly()))
	signature := bootSignature(t, client)
	edited := driftObject(map[string]interface{}{
		"namespaces": []interface{}{"shop", "web"},
	})
	_, err := kwatchConfigs(client).Update(context.Background(), edited,
		metav1.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if got := startedRestarts(t, client, signature); got != 1 {
		t.Fatalf("restarts = %d, want 1 for an edited KwatchConfig", got)
	}
}

func TestWatcherRestartsForConfigDeletedBeforeItStarted(t *testing.T) {
	client := newFakeKwatchConfigClient(driftObject(shopOnly()))
	signature := bootSignature(t, client)
	err := kwatchConfigs(client).Delete(context.Background(), "kwatch",
		metav1.DeleteOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if got := startedRestarts(t, client, signature); got != 1 {
		t.Fatalf("restarts = %d, want 1 for a deleted KwatchConfig", got)
	}
}

// What the overlay applied is still there: nothing to restart for, or
// every start would restart again.
func TestWatcherKeepsRunningWhenStartupConfigIsUnchanged(t *testing.T) {
	cases := map[string]*unstructured.Unstructured{
		"none":            nil,
		"applied":         driftObject(shopOnly()),
		"no spec":         driftObject(nil),
		"refused, as-is":  driftObject(map[string]interface{}{"alert": "x"}),
		"metadata change": driftObject(shopOnly()),
	}
	for name, obj := range cases {
		t.Run(name, func(t *testing.T) {
			client := newFakeKwatchConfigClient()
			if obj != nil {
				client = newFakeKwatchConfigClient(obj)
			}
			signature := bootSignature(t, client)
			if name == "metadata change" {
				relabelled := obj.DeepCopy()
				relabelled.SetLabels(map[string]string{"team": "shop"})
				relabelled.SetResourceVersion("2")
				_, err := kwatchConfigs(client).Update(
					context.Background(), relabelled,
					metav1.UpdateOptions{})
				if err != nil {
					t.Fatal(err)
				}
			}

			if got := startedRestarts(t, client, signature); got != 0 {
				t.Fatalf("restarts = %d, want 0", got)
			}
		})
	}
}

// An edit that startup would refuse must not restart: the restarted
// process would only refuse it again.
func TestWatcherKeepsRunningForInvalidConfigEditedBeforeItStarted(
	t *testing.T,
) {
	client := newFakeKwatchConfigClient(driftObject(shopOnly()))
	signature := bootSignature(t, client)
	invalid := driftObject(map[string]interface{}{"alert": "x"})
	_, err := kwatchConfigs(client).Update(context.Background(), invalid,
		metav1.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if got := startedRestarts(t, client, signature); got != 0 {
		t.Fatalf("restarts = %d, want 0 for an invalid edit", got)
	}
}

func TestStartupSignatureTellsNoneOneAndMany(t *testing.T) {
	none := StartupSignature(nil, false)
	one := StartupSignature([]byte(`{"a":1}`), false)
	refused := StartupSignature([]byte(`{"a":1}`), true)
	many := StartupSignature(nil, true)
	if none != "" || one == "" || one != refused || many == none ||
		many == one {
		t.Fatalf("signatures: none=%q one=%q refused=%q many=%q",
			none, one, refused, many)
	}
	two := []interface{}{driftObject(nil), driftObject(nil)}
	if got, ok := listSignature(two); !ok || got != many {
		t.Fatalf("two objects = %q, %v; want %q", got, ok, many)
	}
	if _, ok := listSignature([]interface{}{struct{}{}}); ok {
		t.Fatal("an unreadable object must not be compared")
	}
}
