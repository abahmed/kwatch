package crdwatch

import (
	"context"
	"errors"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/abahmed/kwatch/internal/config"
)

func TestWatcherStatusReasonsAndObjectBookkeeping(t *testing.T) {
	for message, want := range map[string]string{
		"cache sync failed":        "cache_sync_failed",
		"source not configured":    "source_not_configured",
		"resource not found":       "optional_api_unavailable",
		"unexpected network error": "watcher_failed",
	} {
		if got := safeWatcherReason(message); got != want {
			t.Fatalf("safeWatcherReason(%q) = %q, want %q", message, got, want)
		}
	}
	watcher := &Watcher{
		seen:    make(map[string]string),
		restart: func() {},
	}
	first := &unstructured.Unstructured{Object: map[string]interface{}{}}
	first.SetNamespace("apps")
	first.SetName("config")
	first.SetResourceVersion("1")
	watcher.seedKnown([]interface{}{first, struct{}{}})
	if !watcher.ready || watcher.seen["apps/config"] != "1" {
		t.Fatalf("seeded watcher state = %+v", watcher)
	}
	watcher.deleted(first)
	if _, ok := watcher.seen["apps/config"]; ok {
		t.Fatal("deleted object remained in watcher state")
	}
	if isMissingCRD(errors.New("other")) {
		t.Fatal("ordinary error was classified as missing CRD")
	}
}

func TestWatcherStartHandlesDisabledAndMissingClient(t *testing.T) {
	disabled := config.RuntimeConfigFor(&config.Config{})
	watcher := NewWithClient(
		disabled, nil, "default", 0, func() {}, nil, nil,
	)
	if err := watcher.Start(context.Background()); err != nil {
		t.Fatalf("disabled watcher start = %v", err)
	}
	enabledConfig := &config.Config{}
	enabledConfig.CrdConfig.Enabled = true
	watcher = NewWithClient(
		config.RuntimeConfigFor(enabledConfig), nil, "default", 0,
		func() {}, nil, nil,
	)
	if err := watcher.Start(context.Background()); err == nil {
		t.Fatal("missing dynamic client did not fail")
	}
	if err := watcher.Stop(context.Background()); err != nil {
		t.Fatalf("stop after failed start = %v", err)
	}
	if err := (*Watcher)(nil).Stop(context.Background()); err != nil {
		t.Fatalf("nil watcher stop = %v", err)
	}
}
