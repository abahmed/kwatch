package pipeline

import (
	"fmt"

	"k8s.io/klog/v2"
)

// restore loads the incidents, the startup marker and the fingerprints
// of the previous run. It runs before the loop starts.
func (e *Engine) restore() error {
	startup := &e.announcer.startup
	if e.deps.Store == nil {
		startup.coldStart = true
		return nil
	}
	records, err := e.deps.Store.LoadIncidents()
	if err != nil {
		return fmt.Errorf("pipeline: restore incidents: %w", err)
	}
	marker, found, err := e.deps.Store.LoadStartup()
	if err != nil {
		return fmt.Errorf("pipeline: restore startup marker: %w", err)
	}
	// A store without a marker falls back to the record count: no
	// records means a cold start.
	startup.coldStart = !marker.Complete
	if !found {
		startup.coldStart = len(records) == 0
	}
	if !startup.coldStart {
		startup.summary = marker
		startup.checkSummary = len(marker.Incidents) > 0
	}
	e.deps.Incidents.Restore(records, e.deps.Clock.Now().Add(restoreGrace))
	saved, err := e.deps.Store.LoadFingerprints()
	if err != nil {
		return fmt.Errorf("pipeline: restore fingerprints: %w", err)
	}
	e.storage.saved = saved
	if startup.coldStart {
		// Restore runs before the loop starts, so it may write directly.
		if err := e.deps.Store.SaveStartup(StartupState{}); err != nil {
			klog.ErrorS(err, "pipeline: save startup marker",
				"component", "pipeline")
		}
	}
	return nil
}

// save hands the writer a snapshot of the incidents and, once the initial
// list was compared with the previous run, of the fingerprints. It never
// waits for the write.
func (e *Engine) save() {
	if e.storage.incidents == nil {
		return
	}
	snapshot := storeSnapshot{
		incidents: e.deps.Incidents.Export(), hasIncidents: true,
	}
	if e.storage.reconciled {
		fingerprints := Fingerprints(e.deps.Model, e.deps.Clock.Now())
		e.storage.carried.keep(fingerprints)
		snapshot.fingerprints = fingerprints
	}
	e.storage.incidents.offer(snapshot)
}
