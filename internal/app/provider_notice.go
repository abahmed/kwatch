package app

import (
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
)

// noProvidersNotice says what an installation with no alert provider
// does: it watches and keeps state, and tells nobody. It is empty when a
// provider is configured.
func noProvidersNotice(runtime config.RuntimeConfig) string {
	if len(runtime.Delivery().Providers()) > 0 {
		return ""
	}
	return "no alert provider is configured, so kwatch watches the " +
		"cluster but sends no notification: add one under alert in the " +
		"configuration (credentials go in a Secret and are referenced " +
		"with ${file:/path}); the providers are listed in the " +
		"configuration reference"
}

// logNoProviders warns once per generation when nothing would receive an
// alert. A file with no alert section and no file at all look the same
// from outside, so both warn.
func logNoProviders(runtime config.RuntimeConfig) {
	if notice := noProvidersNotice(runtime); notice != "" {
		klog.Warning(notice)
	}
}
