package app

import (
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/storage"
)

// legacyPushKey is where release candidates stored the dashboard's
// browser-push key and subscriptions. The dashboard no longer pushes, so
// the key is removed from the state file at start.
const legacyPushKey = "dashboard.push"

// dropLegacyPush removes legacyPushKey. A failure is logged and ignored:
// the leftover bytes are inert.
func dropLegacyPush(state *storage.Store) {
	if state == nil {
		return
	}
	err := storage.StateValues[struct{}](state).Delete(legacyPushKey)
	if err != nil {
		klog.ErrorS(err, "could not drop the retired dashboard push state",
			"component", "dashboard-server")
	}
}
