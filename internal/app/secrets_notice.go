package app

import (
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/feature"
)

// communitySecretsUse says what the Community edition does with the
// Secrets it watches. Community runs no detector that reads them, but its
// root-cause engine does: it names a Secret that a failing pod references
// and that does not exist, so the Secret informer is kept.
const communitySecretsUse = ". The Community edition reads Secrets only " +
	"to say in a root cause that a Secret a pod references is missing"

// secretsAccessNotice says what watching Secrets means for the
// ServiceAccount's access. It is empty when Secrets are not watched.
func secretsAccessNotice(
	runtime config.RuntimeConfig, e feature.Entitlements,
) string {
	if !runtime.Application().WatchSecrets {
		return ""
	}
	notice := "watch.secrets is on: kwatch's ServiceAccount can read " +
		"every Secret in the cluster (kwatch keeps only hashes, never " +
		"values)"
	if feature.OrCommunity(e).Edition() == feature.CommunityEdition {
		notice += communitySecretsUse
	}
	return notice + ". To remove that access set watch.secrets=false " +
		"(Helm: --set watch.secrets=false); rbac.mode=least-privilege " +
		"alone narrows other kinds but still lists Secrets"
}

// logSecretsAccess logs the notice once at startup.
func logSecretsAccess(runtime config.RuntimeConfig, e feature.Entitlements) {
	if notice := secretsAccessNotice(runtime, e); notice != "" {
		klog.InfoS(notice, "component", "rbac")
	}
}
