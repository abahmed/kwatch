package app

import (
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery"
	"github.com/abahmed/kwatch/internal/storage"
)

// outboxStore keeps the delivery outbox in the state store's outbox
// bucket, one value per record keyed by its ID. Writes are fenced by the
// store epoch like every other session write.
type outboxStore struct {
	values storage.Keyed[delivery.OutboxRecord]
}

func newOutboxStore(s *storage.Store) outboxStore {
	return outboxStore{values: storage.OutboxValues[delivery.OutboxRecord](s)}
}

// LoadOutbox returns every saved record in key order, which is the order
// the jobs were queued in.
func (o outboxStore) LoadOutbox() ([]delivery.OutboxRecord, error) {
	o.purgeCorrupt()
	var records []delivery.OutboxRecord
	err := o.values.Range("", func(_ string, r delivery.OutboxRecord) error {
		records = append(records, r)
		return nil
	})
	return records, err
}

// purgeCorrupt deletes saved records that no longer decode, logging each
// key once. Such a record can never be sent, and left in the file it would
// not count toward the outbox bound. A failure is logged, not fatal.
func (o outboxStore) purgeCorrupt() {
	removed, err := o.values.PurgeCorrupt()
	if err != nil {
		klog.ErrorS(err, "purge corrupt outbox records",
			"component", "delivery", "operation", "outbox")
		return
	}
	for _, key := range removed {
		klog.ErrorS(nil, "dropped a corrupt outbox record: its page "+
			"cannot be sent", "component", "delivery",
			"operation", "outbox", "key", key)
	}
}

// WriteOutbox stores puts in one transaction, then deletes removes in a
// second. A crash between the two only leaves records that are resent.
func (o outboxStore) WriteOutbox(
	puts []delivery.OutboxRecord, removes []string,
) error {
	items := make(map[string]storage.Item[delivery.OutboxRecord], len(puts))
	for _, record := range puts {
		items[record.ID] = storage.Item[delivery.OutboxRecord]{Value: record}
	}
	if err := o.values.PutAll(items); err != nil {
		return err
	}
	return o.values.DeleteMany(removes)
}

// attachOutbox gives delivery the session's outbox. Delivery still works
// when the outbox cannot be read; it only loses crash protection, which
// /health reports.
func attachOutbox(deps *serverDeps, state *storage.Store) {
	deps.deliveryManager.OnOutboxSick(reportOutboxSick(deps))
	err := deps.deliveryManager.AttachOutbox(newOutboxStore(state))
	if err == nil {
		return
	}
	klog.ErrorS(err, "restore delivery outbox",
		"component", "delivery", "operation", "outbox")
	if deps.healthServer != nil {
		deps.healthServer.SetComponentStatus("delivery-outbox",
			"degraded", "persistence_restore_failed", false)
	}
}

// reportOutboxSick shows on /health whether the outbox writer keeps
// failing. Delivery still works; only crash protection is lost.
func reportOutboxSick(deps *serverDeps) func(bool) {
	return func(sick bool) {
		if deps.healthServer == nil {
			return
		}
		if sick {
			deps.healthServer.SetComponentStatus("delivery-outbox",
				"degraded", "persistence_write_failed", true)
			return
		}
		deps.healthServer.SetComponentStatus("delivery-outbox",
			"running", "", true)
	}
}
