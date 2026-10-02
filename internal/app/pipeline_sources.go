package app

import (
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/scope"
)

// typedSourceConfig configures the typed informer source. dynamic answers
// for the kinds only the dynamic source watches; digestKey keys the Secret
// and ConfigMap digests (see EnsureDigestKey).
func typedSourceConfig(
	deps *serverDeps, submit kube.Submit, maintenance config.MaintenanceConfig,
	dynamic kube.DynamicKinds, digestKey []byte,
) kube.SourceConfig {
	return kube.SourceConfig{
		Client:      deps.clients.Kubernetes,
		Resync:      deps.runtime.Lifecycle().ResyncInterval(),
		Now:         deps.clients.Clock.Now,
		Submit:      submit,
		Maintenance: maintenanceAnnotations(maintenance),
		Dynamic:     dynamic,
		DigestKey:   digestKey,
		// watch.secrets: false leaves Secrets unwatched.
		DisableSecrets: !deps.runtime.Application().WatchSecrets,
	}
}

// kubeletReader is the direct kubelet client, or nil when none was built
// (a nil pointer must not become a non-nil interface).
func kubeletReader(deps *serverDeps) kube.KubeletReader {
	if deps.clients.Kubelet == nil {
		return nil
	}
	return deps.clients.Kubelet
}

// dynamicSourceConfig configures the dynamic source. It honors the same
// maintenance annotations as the typed source.
func dynamicSourceConfig(
	deps *serverDeps, submit kube.Submit, maintenance config.MaintenanceConfig,
) kube.DynamicConfig {
	return kube.DynamicConfig{
		Client:      deps.clients.Dynamic,
		Metadata:    deps.clients.Metadata,
		Discovery:   deps.clients.Discovery,
		Resync:      deps.runtime.Lifecycle().ResyncInterval(),
		Now:         deps.clients.Clock.Now,
		Submit:      submit,
		Maintenance: maintenanceAnnotations(maintenance),
	}
}

// scopedServices hides Services outside the namespace scope from the
// active prober, so automatic Service probing honors the same allowed,
// forbidden and label-selected namespaces as findings do. The probe's
// own excludeNamespaces list still applies on top.
type scopedServices struct {
	inventory.Reader
	scope *scope.Scope
}

func (s scopedServices) Entities(kind inventory.Kind) []inventory.EntityID {
	ids := s.Reader.Entities(kind)
	if kind != kube.KindService {
		return ids
	}
	inScope := ids[:0:0]
	for _, id := range ids {
		if s.scope.NamespaceAllowed(s.Reader, id.Namespace) {
			inScope = append(inScope, id)
		}
	}
	return inScope
}
