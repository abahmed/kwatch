package pipeline

import (
	"sort"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/storage"
)

// Persisted change history. The timeline keeps a one-line text per
// change; this keeps the whole inventory.Change (fields, actor, revision,
// times) so a restart can put it back into the change ring. Records
// are versioned, so a later format can be told from this one.
const (
	// changeStream is the single log stream of the changes bucket. One
	// stream keeps every record in time order, so a restart reads the
	// last day with one range scan.
	changeStream = "cluster"
	// recordVersion is the format of changeRecord and healthyRecord.
	recordVersion = 1
	// maxRestoredChanges bounds how many records a restart reloads: the
	// newest ones, no more than the change ring holds.
	maxRestoredChanges = inventory.DefaultMaxRecentChanges
	// maxPendingChanges bounds changes waiting for the writer. When the
	// store is slow, the oldest are dropped first.
	maxPendingChanges = 2048
	// maxRestoredHealthy bounds how many last-healthy marks a restart
	// reloads.
	maxRestoredHealthy = 2048
	// healthyRetention is how long a last-healthy record stays on disk.
	healthyRetention = 7 * 24 * time.Hour
	// healthyKeyPrefix keys the last-healthy records in the state bucket.
	healthyKeyPrefix = "pipeline.healthy/"
)

// changeRecord is one persisted change.
type changeRecord struct {
	V         int
	Change    inventory.Change
	ChangeSet string `json:",omitempty"`
}

// healthyRecord is the last time a workload was healthy.
type healthyRecord struct {
	V    int
	Mark inventory.HealthMark
}

// changeStore is the part of a store that persists changes. It is
// optional so test stores need not implement it.
type changeStore interface {
	AppendChanges([]changeRecord) error
	// LoadChanges returns the newest limit records with At >= since,
	// oldest first.
	LoadChanges(since time.Time, limit int) ([]changeRecord, error)
}

// healthyStore is the part of a store that persists last-healthy marks.
type healthyStore interface {
	SaveHealthy(map[string]healthyRecord) error
	LoadHealthy(limit int) (map[string]healthyRecord, error)
}

var (
	_ changeStore  = (*diskIncidents)(nil)
	_ healthyStore = (*diskIncidents)(nil)
)

// persistedKinds are the kinds whose changes are kept across restarts:
// workloads, config, services, policy and nodes. Pods, replica sets and
// other controller bookkeeping are left out; they change in bulk and a
// restart rebuilds them from the cluster.
var persistedKinds = map[inventory.Kind]bool{
	kube.KindJob: true, kube.KindHPA: true, kube.KindIngress: true,
}

func init() {
	for kind := range fingerprintAttributes {
		persistedKinds[kind] = true
	}
}

// persistsChange reports whether a change is kept across restarts: a
// real edit, creation or deletion of a persisted kind. A change without
// field edits is a status write.
func persistsChange(change inventory.Change) bool {
	if !persistedKinds[change.Entity.Kind] {
		return false
	}
	return len(change.Fields) > 0 || change.Created || change.Deleted
}

// AppendChanges implements changeStore in one transaction.
func (d *diskIncidents) AppendChanges(records []changeRecord) error {
	batch := make([]storage.Entry[changeRecord], 0, len(records))
	for _, record := range records {
		record.V = recordVersion
		batch = append(batch, storage.Entry[changeRecord]{
			Entity: changeStream, At: record.Change.At, Value: record,
		})
	}
	return storage.ChangeLog[changeRecord](d.store).AppendAll(batch)
}

// LoadChanges implements changeStore. Records of an unknown version
// and records without an entity are skipped.
func (d *diskIncidents) LoadChanges(
	since time.Time, limit int,
) ([]changeRecord, error) {
	var out []changeRecord
	err := storage.ChangeLog[changeRecord](d.store).Range(changeStream,
		since, time.Time{}, func(_ time.Time, record changeRecord) error {
			if record.V >= 1 && record.V <= recordVersion &&
				!record.Change.Entity.IsZero() {
				out = append(out, record)
			}
			return nil
		})
	if over := len(out) - limit; limit > 0 && over > 0 {
		out = out[over:]
	}
	return out, err
}

func (d *diskIncidents) healthy() storage.Keyed[healthyRecord] {
	return storage.StateValues[healthyRecord](d.store)
}

// SaveHealthy implements healthyStore. A record expires
// healthyRetention after the mark.
func (d *diskIncidents) SaveHealthy(
	records map[string]healthyRecord,
) error {
	items := make(map[string]storage.Item[healthyRecord], len(records))
	for entity, record := range records {
		record.V = recordVersion
		items[healthyKeyPrefix+entity] = storage.Item[healthyRecord]{
			Value: record, Expires: record.Mark.At.Add(healthyRetention),
		}
	}
	return d.healthy().PutAll(items)
}

// LoadHealthy implements healthyStore: the newest limit records, keyed
// by entity.
func (d *diskIncidents) LoadHealthy(
	limit int,
) (map[string]healthyRecord, error) {
	all := map[string]healthyRecord{}
	err := d.healthy().Range(healthyKeyPrefix,
		func(key string, record healthyRecord) error {
			if record.V >= 1 && record.V <= recordVersion {
				all[key[len(healthyKeyPrefix):]] = record
			}
			return nil
		})
	if limit <= 0 || len(all) <= limit {
		return all, err
	}
	keys := make([]string, 0, len(all))
	for key := range all {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return all[keys[i]].Mark.At.After(all[keys[j]].Mark.At)
	})
	for _, key := range keys[limit:] {
		delete(all, key)
	}
	return all, err
}
