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
	var records []delivery.OutboxRecord
	err := o.values.Range("", func(_ string, r delivery.OutboxRecord) error {
		records = append(records, r)
		return nil
	})
	return records, err
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
