package app

import (
	"context"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/persistence"
)

// threadRestorer is the narrow slice of the delivery manager needed to hand
// saved conversation ids back to their providers.
type threadRestorer interface {
	RestoreThreads(map[string]map[string]string)
}

// restoreProviderThreads gives providers back the thread ids they were using
// before the restart, but only for incidents that actually came back.
//
// Keeping the rest would be worse than dropping them: an incident that
// resolved while kwatch was down, then recurred a week later, would have its
// alert threaded under a week-old message nobody is reading.
func restoreProviderThreads(
	ctx context.Context,
	persistenceManager persistence.IncidentStore,
	am threadRestorer,
	incidentEngine *incident.Engine,
	aliases incident.KeyAliases,
) error {
	if am == nil {
		return nil
	}
	saved, err := persistenceManager.LoadProviderThreads(ctx)
	if err != nil {
		klog.ErrorS(err, "failed to restore provider thread state")
		return err
	}
	if len(saved) == 0 {
		return nil
	}
	live := make(map[string]bool)
	for key := range incidentEngine.ActiveNotificationKeys() {
		live[string(key)] = true
	}
	kept := make(map[string]map[string]string, len(saved))
	total := 0
	for provider, threads := range saved {
		for rawKey, ts := range threads {
			key := incident.ResolveKeyAlias(
				aliases, model.IncidentKey(rawKey),
			)
			key = incident.CanonicalIncidentKey(key)
			keyString := string(key)
			if !live[keyString] {
				continue
			}
			if kept[provider] == nil {
				kept[provider] = make(map[string]string)
			}
			kept[provider][keyString] = ts
			total++
		}
	}
	if total == 0 {
		return nil
	}
	am.RestoreThreads(kept)
	klog.InfoS("restored provider threads from configmap", "count", total)
	return nil
}

// restoreEngineState reinstates the correlation bookkeeping that is neither
// an incident nor a group. Call it after the incidents are back: entries for
// incidents that did not return are dropped rather than resurrected.
func restoreEngineState(
	ctx context.Context,
	persistenceManager persistence.IncidentStore,
	incidentEngine *incident.Engine,
) error {
	engine, err := persistenceManager.LoadEngineState(ctx)
	if err != nil {
		klog.ErrorS(err, "failed to restore engine state from configmap")
		return err
	}
	incidentEngine.RestoreEngineState(engine)
	return nil
}
