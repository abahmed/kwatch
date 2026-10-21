package app

import (
	"encoding/json"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/storage"
)

// stateOverlayLastGood is the state key that holds the last KwatchConfig
// spec (as JSON text) that applied cleanly. An empty value means there is
// none: the object was deleted on purpose.
const stateOverlayLastGood = "config.overlay.lastgood"

// readLastGood reads the last good spec for a start that must fall back to
// it. The overlay is read while the process boots, before the state lock
// is held, so the file is opened read-only without claiming it: nothing is
// written, a missing or unusable file reads as no spec, and the file is
// closed again at once, so the leader's own open is not delayed. It runs
// only when the KwatchConfig was refused, so a failure is logged once, at
// the default verbosity, with its reason: it says why the spec is missing.
func readLastGood(path string, now func() time.Time) ([]byte, bool) {
	store, err := storage.Open(path, storage.Options{
		Now: now, DeferRepair: true,
	})
	if err != nil {
		logLastGoodUnavailable(lastGoodOpenReason(err), err)
		return nil, false
	}
	defer func() { _ = store.Close() }()
	var spec string
	found, err := diskState{store: store}.get(stateOverlayLastGood, &spec)
	switch {
	case err != nil:
		logLastGoodUnavailable("state file cannot be read", err)
	case !found || spec == "":
		// Nothing was kept, or the object was deleted on purpose.
	case !json.Valid([]byte(spec)):
		logLastGoodUnavailable("kept spec is not valid JSON", nil)
	default:
		return []byte(spec), true
	}
	return nil, false
}

// lastGoodOpenReason words why the state file did not open: a lock held
// for the whole open timeout is the common, transient case.
func lastGoodOpenReason(err error) string {
	if storage.IsLockTimeout(err) {
		return "state file is locked by another process"
	}
	return "state file cannot be opened"
}

func logLastGoodUnavailable(reason string, err error) {
	values := []interface{}{"component", "crd-watcher",
		"operation", "startup", "reason", reason}
	if err != nil {
		values = append(values, "error", err.Error())
	}
	klog.InfoS("cannot use the last good KwatchConfig", values...)
}

// rememberOverlay keeps, in the state file, what the startup KwatchConfig
// turned out to be: a spec that applied cleanly becomes the last good one,
// and no object (deleted on purpose) clears it, so a later invalid one
// cannot bring an old spec back. A refused spec changes nothing. The
// leader calls it once the state file is open and claimed.
func rememberOverlay(deps *serverDeps, disk diskState) {
	o := deps.overlay
	if o.invalid {
		return
	}
	want := string(o.applied)
	var have string
	if _, err := disk.get(stateOverlayLastGood, &have); err != nil {
		klog.ErrorS(err, "read the last good KwatchConfig",
			"component", "crd-watcher")
		return
	}
	if have == want {
		return
	}
	if err := disk.put(stateOverlayLastGood, want); err != nil {
		klog.ErrorS(err, "keep the last good KwatchConfig",
			"component", "crd-watcher")
	}
}
