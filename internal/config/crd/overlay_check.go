package crd

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
)

// overlayInvalidReason is the bounded health reason for a KwatchConfig
// that kwatch refused to apply.
const overlayInvalidReason = "config_overlay_invalid"

// validateOverlay applies obj to a fresh copy of the mounted configuration
// and reports whether kwatch could start with the result. Restarting on an
// invalid edit would only crash-loop: the startup overlay would reject the
// same object. objects is how many KwatchConfigs the namespace now holds;
// startup accepts at most one.
func (w *Watcher) validateOverlay(obj interface{}, objects int) error {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok || w.loadBase == nil {
		return nil
	}
	if objects > 1 {
		return fmt.Errorf(
			"%w: expected at most one KwatchConfig in namespace %q",
			ErrInvalidOverlay, w.namespace,
		)
	}
	base, err := w.loadBase()
	if err != nil {
		// The mounted file is not the KwatchConfig's fault; keep the old
		// behavior and let the restart report it.
		klog.ErrorS(err, "cannot reload the mounted configuration to "+
			"check a KwatchConfig change", "component", "crd-watcher")
		return nil
	}
	return ApplyOverlay(base, u)
}

// rejectOverlay keeps the running configuration after an invalid
// KwatchConfig change. The object's version is remembered so a resync does
// not report it again; the next edit is checked afresh.
func (w *Watcher) rejectOverlay(obj interface{}, err error) {
	name := ""
	if accessor, accessErr := meta.Accessor(obj); accessErr == nil {
		name = accessor.GetNamespace() + "/" + accessor.GetName()
		w.mu.Lock()
		w.seen[name] = accessor.GetResourceVersion()
		w.mu.Unlock()
	}
	klog.ErrorS(err, "KwatchConfig change rejected; kwatch keeps running "+
		"with its current configuration until the object is fixed",
		"component", "crd-watcher", "operation", "validate",
		"kwatchconfig", name, "reason", overlayInvalidReason)
	w.reportError(err)
}
