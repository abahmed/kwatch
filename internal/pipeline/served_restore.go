package pipeline

import (
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Restored served memory is checked against the cluster once the
// workload's kind is fully listed. kwatch forgets a workload's memory
// when it sees the workload deleted, so a deletion while it was down
// leaves the memory behind. A workload absent from the full list was
// deleted; one created after a remembered sighting was deleted and
// created again under its name. A template that did not change gives
// its revisions the same names, so the old object's memory would speak
// for the new one and back a rollback to a revision it never ran.
// Either way that memory is dropped, in the model and on disk.

// servedCheck tracks the workloads whose served memory was restored
// and is not yet checked: pending until their kind is fully listed,
// then listed until the drain that follows has run.
type servedCheck struct {
	pending map[inventory.EntityID]bool
	listed  []inventory.EntityID
}

// add marks a workload whose served memory was restored.
func (r *servedCheck) add(id inventory.EntityID) {
	if r.pending == nil {
		r.pending = map[inventory.EntityID]bool{}
	}
	r.pending[id] = true
}

// listedServed moves the restored workloads whose kind synced reports
// fully listed to be checked after the next drain. It runs before the
// drain: a kind reads as synced only after its source submitted the
// whole list, so once drained the model holds every listed workload.
func (h *historyRecorder) listedServed(synced func(inventory.Kind) bool) {
	for id := range h.restored.pending {
		if kindSynced(synced, id.Kind) {
			h.restored.listed = append(h.restored.listed, id)
			delete(h.restored.pending, id)
		}
	}
}

// checkRestoredServed checks, after the drain, the restored workloads
// listedServed moved: the memory of one that is absent goes, and of
// one that is present the revisions last seen before it was created.
func (h *historyRecorder) checkRestoredServed() {
	for _, id := range h.restored.listed {
		if !h.model.Exists(id) {
			h.dropServed(id)
			continue
		}
		created, ok := h.model.Attribute(id, kube.AttrCreated)
		if !ok {
			continue
		}
		kept, dropped := h.model.ForgetServedBefore(id,
			created.Value.AsTime())
		switch {
		case !dropped || h.writer == nil:
		case len(kept) == 0:
			h.writer.forgetServed(id)
		default:
			h.writer.addServed(id, kept)
		}
	}
	h.restored.listed = nil
}

// dropServed forgets a workload's served revisions, in the model and
// on disk.
func (h *historyRecorder) dropServed(id inventory.EntityID) {
	h.model.ForgetServed(id)
	if h.writer != nil {
		h.writer.forgetServed(id)
	}
}
