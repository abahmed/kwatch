package app

import (
	"context"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/persistence"
)

// restoreIncidents loads previously persisted incidents back into memory.
func restoreIncidents(
	ctx context.Context,
	persistenceManager persistence.IncidentStore,
	incidentEngine *incident.Engine,
	allowed func(string) bool,
) {
	persisted, err := persistenceManager.LoadPersistedIncidents(ctx)
	if err != nil {
		// Restarting without correlation memory is recoverable, but it may
		// announce already-broken resources again as new incidents.
		klog.ErrorS(err, "failed to restore incidents from configmap")
		return
	}
	incidentEngine.FilterBaseline(allowed)
	if len(persisted) == 0 {
		return
	}
	restored := make(map[model.IncidentKey]*model.Incident, len(persisted))
	for i := range persisted {
		inc := persisted[i].ToIncident()
		if inc.Namespace != "" && allowed != nil && !allowed(inc.Namespace) {
			continue
		}
		restored[inc.Key] = inc
	}
	incidentEngine.RestoreIncidents(restored)
	klog.InfoS("restored incidents from configmap", "count", len(persisted))

	// Groups are restored second so absent incidents can be discarded safely.
	groups, err := persistenceManager.LoadPersistedGroups(ctx)
	if err != nil {
		klog.ErrorS(err, "failed to restore smart group state from configmap")
		return
	}
	incidentEngine.RestoreGroups(groups)
}
