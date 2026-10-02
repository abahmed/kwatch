package pipeline

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/storage"
)

// HistoryStore persists the per-entity timeline and workload baselines.
// The incident store from NewIncidentStore implements it; the engine
// uses it when its Store does.
type HistoryStore interface {
	AppendTimeline([]TimelineEntry) error
	// LoadTimeline returns entity's entries with since <= At < until,
	// ordered by SortTimeline. A zero bound is open.
	LoadTimeline(entity string, since, until time.Time) ([]TimelineEntry, error)
	LoadBaselines() (map[string]inventory.Baseline, error)
	SaveBaselines(map[string]inventory.Baseline) error
}

var _ HistoryStore = (*diskIncidents)(nil)

func (d *diskIncidents) timeline() storage.Log[TimelineEntry] {
	return storage.TimelineLog[TimelineEntry](d.store)
}

func (d *diskIncidents) baselines() storage.Keyed[inventory.Baseline] {
	return storage.BaselineValues[inventory.Baseline](d.store)
}

// AppendTimeline implements HistoryStore in one transaction.
func (d *diskIncidents) AppendTimeline(entries []TimelineEntry) error {
	batch := make([]storage.Entry[TimelineEntry], 0, len(entries))
	for _, entry := range entries {
		batch = append(batch, storage.Entry[TimelineEntry]{
			Entity: entry.Entity, At: entry.At, Value: entry,
		})
	}
	return d.timeline().AppendAll(batch)
}

// LoadTimeline implements HistoryStore.
func (d *diskIncidents) LoadTimeline(
	entity string, since, until time.Time,
) ([]TimelineEntry, error) {
	var out []TimelineEntry
	err := d.timeline().Range(entity, since, until,
		func(_ time.Time, entry TimelineEntry) error {
			out = append(out, entry)
			return nil
		})
	SortTimeline(out)
	return out, err
}

// LoadBaselines implements HistoryStore. Corrupt values are skipped.
func (d *diskIncidents) LoadBaselines() (map[string]inventory.Baseline, error) {
	out := map[string]inventory.Baseline{}
	err := d.baselines().Range("",
		func(key string, value inventory.Baseline) error {
			out[key] = value
			return nil
		})
	return out, err
}

// SaveBaselines implements HistoryStore; baselines not in values stay.
func (d *diskIncidents) SaveBaselines(
	values map[string]inventory.Baseline,
) error {
	items := make(map[string]storage.Item[inventory.Baseline], len(values))
	for key, value := range values {
		items[key] = storage.Item[inventory.Baseline]{Value: value}
	}
	return d.baselines().PutAll(items)
}

// DeleteBaselines removes the baselines of keys in one transaction, for
// example the expired ones inventory reports. It is not part of
// HistoryStore so that test doubles need not implement it.
func (d *diskIncidents) DeleteBaselines(keys []string) error {
	return d.baselines().DeleteMany(keys)
}
