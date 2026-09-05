package crdwatch

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestRestartForLateConfig(t *testing.T) {
	restarts := 0
	watcher := &Watcher{restart: func() { restarts++ }}

	if watcher.restartForLateConfig(0) {
		t.Fatal("empty initial list must not restart")
	}
	if !watcher.restartForLateConfig(1) {
		t.Fatal("late initial config must request restart")
	}
	watcher.restartForLateConfig(1)
	if restarts != 1 {
		t.Fatalf("restart must happen once, got %d", restarts)
	}
}

func TestRestartForFirstConfigAddedAfterReady(t *testing.T) {
	restarts := 0
	watcher := &Watcher{
		restart: func() { restarts++ },
		seen:    make(map[string]string),
		ready:   true,
	}
	obj := &unstructured.Unstructured{Object: map[string]interface{}{}}
	obj.SetNamespace("kwatch")
	obj.SetName("kwatch")
	obj.SetResourceVersion("1")

	watcher.changed(obj)
	if restarts != 1 {
		t.Fatalf("first post-startup config must restart, got %d", restarts)
	}
	watcher.changed(obj)
	if restarts != 1 {
		t.Fatalf("unchanged config must not restart again, got %d", restarts)
	}

	updated := obj.DeepCopy()
	updated.SetResourceVersion("2")
	watcher.changed(updated)
	if restarts != 1 {
		t.Fatalf("restart must remain once-only, got %d", restarts)
	}
}

func TestWatcherIgnoresMalformedObjects(t *testing.T) {
	watcher := &Watcher{
		restart: func() { t.Fatal("malformed object must not restart") },
		seen:    make(map[string]string),
		ready:   true,
	}
	watcher.changed(struct{}{})
}
