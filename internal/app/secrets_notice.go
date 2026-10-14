package app

import (
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
)

// secretsAccessNotice says what watching Secrets means for the
// ServiceAccount's access. It is empty when Secrets are not watched.
func secretsAccessNotice(runtime config.RuntimeConfig) string {
	if !runtime.Application().WatchSecrets {
		return ""
	}
	return "watch.secrets is on: kwatch's ServiceAccount can read every " +
		"Secret in the cluster (kwatch keeps only hashes, never values). " +
		"To remove that access set watch.secrets=false (Helm: --set " +
		"watch.secrets=false); rbac.mode=least-privilege alone narrows " +
		"other kinds but still lists Secrets"
}

// logSecretsAccess logs the notice once at startup.
func logSecretsAccess(runtime config.RuntimeConfig) {
	if notice := secretsAccessNotice(runtime); notice != "" {
		klog.InfoS(notice, "component", "rbac")
	}
}
