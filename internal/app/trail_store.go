package app

import (
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery"
	"github.com/abahmed/kwatch/internal/storage"
)

// LoadTrails returns the saved send trails of the incidents the state
// file still holds. The trail of an incident that was forgotten (its
// record expired or was dropped) is deleted, as are records that no
// longer decode.
func (o outboxStore) LoadTrails() ([]delivery.TrailRecord, error) {
	if _, err := o.trails.PurgeCorrupt(); err != nil {
		klog.ErrorS(err, "purge corrupt delivery trails",
			"component", "delivery", "operation", "trail")
	}
	var saved []delivery.TrailRecord
	err := o.trails.Range("", func(_ string, r delivery.TrailRecord) error {
		saved = append(saved, r)
		return nil
	})
	if err != nil {
		return nil, err
	}
	var kept []delivery.TrailRecord
	var forgotten []string
	for _, r := range saved {
		_, known, err := o.incidents.Get(r.Key)
		if err != nil {
			return nil, err
		}
		if known {
			kept = append(kept, r)
		} else {
			forgotten = append(forgotten, r.Key)
		}
	}
	if err := o.trails.DeleteMany(forgotten); err != nil {
		klog.ErrorS(err, "delete forgotten delivery trails",
			"component", "delivery", "operation", "trail")
	}
	return kept, nil
}

// WriteTrails stores puts, then deletes removes, like WriteOutbox.
func (o outboxStore) WriteTrails(
	puts []delivery.TrailRecord, removes []string,
) error {
	items := make(map[string]storage.Item[delivery.TrailRecord], len(puts))
	for _, record := range puts {
		items[record.Key] = storage.Item[delivery.TrailRecord]{Value: record}
	}
	if err := o.trails.PutAll(items); err != nil {
		return err
	}
	return o.trails.DeleteMany(removes)
}
