package app

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/storage"
)

// usageWindow is how long a stored usage history outlives its last
// reading: the 24 hours its peaks cover. Past it the compactor deletes
// the record even when no kwatch runs to prune it.
const usageWindow = 24 * time.Hour

// usageStore keeps the container memory and CPU history of the kubelet
// stats poller in the state store's usage bucket, one record per
// container keyed by its entity ID. The mirror writes only the records
// that changed and deletes the ones the poller no longer holds.
type usageStore struct {
	values storage.Keyed[kube.UsageRecord]
	mirror *storage.Mirror[kube.UsageRecord]
}

// newUsageStore returns the store, or nil without a state file: the
// history then lives for the session only.
func newUsageStore(s *storage.Store) kube.UsageStore {
	if s == nil {
		return nil
	}
	values := storage.UsageValues[kube.UsageRecord](s)
	return usageStore{values: values, mirror: storage.NewMirror(values)}
}

// LoadUsage returns every stored record whose key is an entity ID.
func (u usageStore) LoadUsage() (
	map[inventory.EntityID]kube.UsageRecord, error,
) {
	out := map[inventory.EntityID]kube.UsageRecord{}
	err := u.values.Range("", func(key string, r kube.UsageRecord) error {
		if id, ok := inventory.ParseEntityID(key); ok {
			out[id] = r
		}
		return nil
	})
	return out, err
}

// SaveUsage makes records the whole content of the bucket.
func (u usageStore) SaveUsage(
	records map[inventory.EntityID]kube.UsageRecord,
) error {
	items := make(map[string]storage.Item[kube.UsageRecord], len(records))
	for id, r := range records {
		items[id.String()] = storage.Item[kube.UsageRecord]{
			Value: r, Expires: r.Seen.Add(usageWindow),
		}
	}
	_, err := u.mirror.Replace(items)
	return err
}
