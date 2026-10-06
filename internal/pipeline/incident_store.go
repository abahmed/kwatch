package pipeline

import (
	"sync"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/pipeline/announce"
	"github.com/abahmed/kwatch/internal/storage"
)

// resolvedHistory is how long a resolved incident stays on disk. It
// matches how long the manager remembers it for recurrence, so the
// compactor removes a record once the manager would drop it anyway.
const resolvedHistory = incident.DefaultRemember

// fingerprintInterval is how often fingerprints are written. They change
// with every watched object update but are only read at the next start,
// so writing them every batch would rewrite thousands of keys for no
// reader. The latest set is also written when the store closes.
const fingerprintInterval = 5 * time.Minute

// diskIncidents stores incident records in the state file, one key per
// incident, and deletes records the manager no longer holds. Saves write
// only the keys whose value changed (see storage.Mirror).
type diskIncidents struct {
	store        *storage.Store
	incidents    *storage.Mirror[incident.Record]
	fingerprints *storage.Mirror[any]
	state        storage.Keyed[announce.StartupState]

	mu sync.Mutex
	// pending is the newest fingerprint set not yet written; nil when
	// everything is written.
	pending map[string]any
	// lastFingerprints is when fingerprints were last written.
	lastFingerprints time.Time
}

// NewIncidentStore persists incidents in s. Held-back fingerprints are
// written when s closes.
func NewIncidentStore(s *storage.Store) IncidentStore {
	d := &diskIncidents{
		store: s,
		incidents: storage.NewMirror(
			storage.IncidentRecords[incident.Record](s)),
		fingerprints: storage.NewMirror(storage.FingerprintValues[any](s)),
		state:        storage.StateValues[announce.StartupState](s),
	}
	s.OnClose(d.flushOnClose)
	return d
}

// LoadIncidents implements IncidentStore. Corrupt and expired records
// are skipped by the store.
func (d *diskIncidents) LoadIncidents() ([]incident.Record, error) {
	var out []incident.Record
	err := storage.IncidentRecords[incident.Record](d.store).Range("",
		func(_ string, record incident.Record) error {
			out = append(out, record)
			return nil
		})
	return out, err
}

// SaveIncidents implements IncidentStore in one transaction that writes
// only new or changed records and deletes dropped ones. A resolved
// record expires resolvedHistory after it resolved.
func (d *diskIncidents) SaveIncidents(records []incident.Record) error {
	items := make(map[string]storage.Item[incident.Record], len(records))
	for _, record := range records {
		items[record.ID] = storage.Item[incident.Record]{
			Value: record, Expires: expiry(record),
		}
	}
	_, err := d.incidents.Replace(items)
	return err
}

func expiry(record incident.Record) time.Time {
	if record.State != incident.Resolved || record.Resolved.IsZero() {
		return time.Time{}
	}
	return record.Resolved.Add(resolvedHistory)
}

// LoadFingerprints implements IncidentStore. A value that is not a
// string is skipped.
func (d *diskIncidents) LoadFingerprints() (map[string]string, error) {
	out := map[string]string{}
	err := storage.FingerprintValues[any](d.store).Range("",
		func(key string, value any) error {
			if digest, ok := value.(string); ok {
				out[key] = digest
			}
			return nil
		})
	return out, err
}

// startupKey holds the startup summary marker in the state bucket.
const startupKey = "pipeline.startup"

// LoadStartup implements IncidentStore.
func (d *diskIncidents) LoadStartup() (announce.StartupState, bool, error) {
	return d.state.Get(startupKey)
}

// SaveStartup implements IncidentStore.
func (d *diskIncidents) SaveStartup(state announce.StartupState) error {
	return d.state.Put(startupKey, state)
}

// SaveFingerprints implements IncidentStore. It writes at most once per
// fingerprintInterval; a newer set saved in between replaces the held
// one and is written by the next save after the interval, or at close.
func (d *diskIncidents) SaveFingerprints(values map[string]any) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.pending = values
	now := d.store.Now()
	if !d.lastFingerprints.IsZero() &&
		now.Sub(d.lastFingerprints) < fingerprintInterval {
		return nil
	}
	return d.writeFingerprintsLocked(now)
}

// FlushFingerprints writes held-back fingerprints now.
func (d *diskIncidents) FlushFingerprints() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.writeFingerprintsLocked(d.store.Now())
}

func (d *diskIncidents) writeFingerprintsLocked(now time.Time) error {
	if d.pending == nil {
		return nil
	}
	items := make(map[string]storage.Item[any], len(d.pending))
	for key, value := range d.pending {
		items[key] = storage.Item[any]{Value: value}
	}
	if _, err := d.fingerprints.Replace(items); err != nil {
		return err
	}
	d.pending, d.lastFingerprints = nil, now
	return nil
}

// flushOnClose is the final fingerprint write. A deposed leader's write
// is fenced; either way the error is only logged, because the store is
// closing.
func (d *diskIncidents) flushOnClose() {
	if err := d.FlushFingerprints(); err != nil {
		klog.ErrorS(err, "pipeline: final fingerprint write failed",
			"component", "pipeline", "operation", "save",
			"data", "fingerprints")
	}
}
