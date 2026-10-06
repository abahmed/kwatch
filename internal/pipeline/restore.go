package pipeline

import (
	"fmt"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/pipeline/announce"
)

// restore loads the incidents, the startup marker and the fingerprints
// of the previous run. It runs before the loop starts.
func (e *Engine) restore() error {
	startup := &e.announcer.collect.Startup
	began := e.deps.Clock.Now()
	if e.deps.Store == nil {
		startup.ColdStart = true
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
	grace := e.deps.Clock.Now().Add(restoreGrace)
	startup.Restore(marker, found, len(records), grace)
	e.deps.Incidents.Restore(records, grace)
	e.announcer.collect.RestoreDigest()
	saved, err := e.deps.Store.LoadFingerprints()
	if err != nil {
		return fmt.Errorf("pipeline: restore fingerprints: %w", err)
	}
	e.storage.saved = saved
	took := e.deps.Clock.Now().Sub(began)
	klog.InfoS("restored state", "component", "pipeline",
		"operation", "restore", "incidents", len(records),
		"fingerprints", len(saved), "coldStart", startup.ColdStart,
		"durationMs", took.Milliseconds())
	if startup.ColdStart {
		// Restore runs before the loop starts, so it may write directly.
		if err := e.deps.Store.SaveStartup(announce.StartupState{}); err != nil {
			klog.ErrorS(err, "pipeline: save startup marker",
				"component", "pipeline")
		}
	}
	return nil
}

// save hands the writer a snapshot of the incidents and, once the initial
// list was compared with the previous run, of the fingerprints. It never
// waits for the write. It always includes the fingerprints; the loop uses
// saveAt, which builds them only when the store would write them.
func (e *Engine) save() {
	e.saveAt(e.deps.Clock.Now(), true)
}

// saveAt is save at now. The fingerprints cover every tracked object, so
// they are built only when the store would write them, once per
// fingerprintInterval, or when force is set: the store holds back the
// newer ones until its interval is over, and a snapshot built in between
// would be thrown away. The final save at shutdown forces them, so the
// next start compares against the latest state.
func (e *Engine) saveAt(now time.Time, force bool) {
	if e.storage.incidents == nil {
		return
	}
	snapshot := storeSnapshot{
		incidents: e.deps.Incidents.Export(), hasIncidents: true,
	}
	if e.storage.reconciled && (force || e.storage.fingerprintsDue(now)) {
		fingerprints := Fingerprints(e.deps.Model, now)
		e.storage.carried.keep(fingerprints)
		snapshot.fingerprints = fingerprints
		e.storage.fingerprinted = now
	}
	e.storage.incidents.offer(snapshot)
}
