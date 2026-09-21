package app

import (
	"context"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/persistence"
)

// restoreIncidents loads previously persisted incidents back into memory.
func restoreIncidents(
	ctx context.Context,
	persistenceManager persistence.IncidentStore,
	incidentEngine *incident.Engine,
	allowed func(string) bool,
) (incident.KeyAliases, error) {
	persisted, err := persistenceManager.LoadPersistedIncidents(ctx)
	if err != nil {
		// Restarting without correlation memory is recoverable, but it may
		// announce already-broken resources again as new incidents.
		klog.ErrorS(err, "failed to restore incidents from configmap")
		return nil, err
	}
	incidentEngine.FilterBaseline(allowed)
	if len(persisted) == 0 {
		return nil, nil
	}
	restored, aliases := incident.RestoreIncidentRecords(persisted)
	for key, inc := range restored {
		if inc.Namespace != "" && allowed != nil && !allowed(inc.Namespace) {
			delete(restored, key)
			continue
		}
	}
	incidentEngine.RestoreIncidents(restored)
	klog.InfoS("restored incidents from configmap", "count", len(persisted))

	return aliases, nil
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
