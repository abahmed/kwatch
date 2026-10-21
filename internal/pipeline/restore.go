package pipeline

import (
	"fmt"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/pipeline/announce"
)

// restore loads the incidents, the startup marker and the fingerprints
// of the previous run. It runs before the loop starts.
func (e *Engine) restore() error {
	startup := &e.announcer.collect.Startup
	began := e.deps.Clock.Now()
	if e.deps.Store == nil || e.deps.baselineStart() {
		startup.ColdStart = true
		if e.deps.baselineStart() {
			startup.HoldUntilSync()
			return e.restoreAnnounced()
		}
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
	e.storage.history.checks.restore(e.deps.Store)
	e.storage.history.unexplained.restore(e.deps.Store)
	took := e.deps.Clock.Now().Sub(began)
	klog.InfoS("restored state", "component", "pipeline",
		"operation", "restore", "incidents", len(records),
		"fingerprints", len(saved), "coldStart", startup.ColdStart,
		"durationMs", took.Milliseconds())
	if startup.ColdStart {
		startup.HoldUntilSync()
		// Restore runs before the loop starts, so it may write directly.
		if err := e.deps.Store.SaveStartup(announce.StartupState{}); err != nil {
			klog.ErrorS(err, "pipeline: save startup marker",
				"component", "pipeline")
		}
	}
	return nil
}

// restoreAnnounced is the restore of a baseline (Community) start: only
// the incidents announced before the restart and still open come back, so
// kwatch can send their resolves. The failures that exist now are the
// baseline, whatever the earlier run knew about them.
func (e *Engine) restoreAnnounced() error {
	if e.deps.Store == nil {
		return nil
	}
	records, err := e.deps.Store.LoadIncidents()
	if err != nil {
		return fmt.Errorf("pipeline: restore incidents: %w", err)
	}
	kept := openAnnounced(records, nil)
	if len(kept) == 0 {
		return nil
	}
	e.deps.Incidents.Restore(kept, e.deps.Clock.Now().Add(restoreGrace))
	klog.InfoS("restored announced incidents", "component", "pipeline",
		"operation", "restore", "incidents", len(kept), "coldStart", true)
	return nil
}

// openAnnounced keeps the records a baseline start saves and restores:
// open incidents that someone was told about. skip names incidents to
// leave out; the baselined ones are, because nobody heard of them, and a
// resolve of one would be news from nowhere.
func openAnnounced(
	records []incident.Record, skip func(id string) bool,
) []incident.Record {
	var kept []incident.Record
	for _, r := range records {
		open := r.State == incident.Open || r.State == incident.Recovering
		if open && !r.Announced.IsZero() && !r.Unheard &&
			(skip == nil || !skip(r.ID)) {
			kept = append(kept, r)
		}
	}
	return kept
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
	records := e.deps.Incidents.Export()
	if e.storage.openOnly {
		records = openAnnounced(records,
			e.announcer.collect.Startup.Baselined)
	}
	snapshot := storeSnapshot{incidents: records, hasIncidents: true}
	if !e.storage.openOnly && e.storage.reconciled &&
		(force || e.storage.fingerprintsDue(now)) {
		fingerprints := Fingerprints(e.deps.Model, now)
		e.storage.carried.keep(fingerprints)
		snapshot.fingerprints = fingerprints
		e.storage.fingerprinted = now
	}
	e.storage.incidents.offer(snapshot)
}
