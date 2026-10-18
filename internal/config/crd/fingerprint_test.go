package crd

import "testing"

// Re-applying an identical manifest moves resourceVersion and metadata
// but not the spec: kwatch must not restart. A spec edit still does.
func TestWatcherIgnoresMetadataOnlyKwatchConfigChange(t *testing.T) {
	restarts := 0
	spec := func(ns string) map[string]interface{} {
		return map[string]interface{}{"namespaces": []interface{}{ns}}
	}
	watcher := &Watcher{
		restart:  func() { restarts++ },
		seen:     make(map[string]string),
		loadBase: freshBase,
	}
	watcher.seedKnown([]interface{}{kwatchConfig("1", spec("apps"))})

	reapplied := kwatchConfig("2", spec("apps"))
	reapplied.SetAnnotations(map[string]string{
		"kubectl.kubernetes.io/last-applied-configuration": "{}"})
	watcher.changed(reapplied)
	if restarts != 0 {
		t.Fatalf("metadata-only change restarted kwatch %d times",
			restarts)
	}

	watcher.changed(kwatchConfig("3", spec("shop")))
	if restarts != 1 {
		t.Fatalf("spec change restarts = %d, want 1", restarts)
	}
}
