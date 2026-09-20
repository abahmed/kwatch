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
) error {
	persisted, err := persistenceManager.LoadPersistedIncidents(ctx)
	if err != nil {
		// Restarting without correlation memory is recoverable, but it may
		// announce already-broken resources again as new incidents.
		klog.ErrorS(err, "failed to restore incidents from configmap")
		return err
	}
	incidentEngine.FilterBaseline(allowed)
	if len(persisted) == 0 {
		return nil
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

	return nil
}

// restoreGroups loads smart-group state after incidents have been restored.
// Keeping the stores separate makes each startup operation visible in the
// migration report and keeps restore ordering explicit.
func restoreGroups(
	ctx context.Context,
	persistenceManager persistence.IncidentStore,
	incidentEngine *incident.Engine,
) error {
	groups, err := persistenceManager.LoadPersistedGroups(ctx)
	if err != nil {
		klog.ErrorS(err, "failed to restore smart group state from configmap")
		return err
	}
	incidentEngine.RestoreGroups(groups)
	return nil
}
