package crdwatch

import (
	"encoding/json"
	"strings"
)

// StatusJSON implements a safe diagnostic boundary for the CRD watcher.
func (w *Watcher) StatusJSON() ([]byte, error) {
	return json.Marshal(w.Status())
}

func safeWatcherReason(message string) string {
	message = strings.ToLower(message)
	switch {
	case strings.Contains(message, "cache") &&
		strings.Contains(message, "sync"):
		return "cache_sync_failed"
	case strings.Contains(message, "not found"):
		return "optional_api_unavailable"
	case strings.Contains(message, "discovery"):
		return "discovery_failed"
	default:
		return "watcher_failed"
	}
}
