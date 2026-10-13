package pipeline

import (
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// restoreChanges reloads the last day of persisted changes and the
// last-healthy marks into the model, with their original times and
// actors. It runs before the sources report, so before the changes made
// while kwatch was down are found. A store without these records, such
// as a state file from an older version, restores nothing.
func (h *historyRecorder) restoreChanges() {
	now := h.now()
	var since time.Time
	if !now.IsZero() {
		since = now.Add(-inventory.ChangeRetention)
	}
	restored := 0
	if store, ok := h.store.(changeStore); ok {
		records, err := store.LoadChanges(since, maxRestoredChanges)
		if err != nil {
			klog.ErrorS(err, "pipeline: restore changes; starting empty",
				"component", "pipeline", "operation", "restore")
		}
		changes := make([]inventory.Change, 0, len(records))
		for _, record := range records {
			changes = append(changes, record.Change)
		}
		restored = h.model.RestoreChanges(changes)
	}
	marks := h.restoreHealthy(since)
	if restored > 0 || marks > 0 {
		klog.InfoS("restored change history", "component", "pipeline",
			"operation", "restore", "changes", restored, "healthyMarks", marks)
	}
}

// restoreHealthy puts the saved last-healthy marks no older than since
// back into the model and returns how many it restored.
func (h *historyRecorder) restoreHealthy(since time.Time) int {
	store, ok := h.store.(healthyStore)
	if !ok {
		return 0
	}
	saved, err := store.LoadHealthy(maxRestoredHealthy)
	if err != nil {
		klog.ErrorS(err, "pipeline: restore healthy marks; starting empty",
			"component", "pipeline", "operation", "restore")
	}
	restored := 0
	for key, record := range saved {
		id, ok := inventory.ParseEntityID(key)
		if !ok || record.Mark.Failing || record.Mark.At.Before(since) {
			continue
		}
		h.model.RestoreHealth(id, record.Mark)
		restored++
	}
	return restored
}

// saveHealthy queues a workload's healthy mark for the store, with the
// revision the workload is at.
func (h *historyRecorder) saveHealthy(id inventory.EntityID, at time.Time) {
	if h.writer == nil || !incident.IsWorkload(id.Kind) {
		return
	}
	mark := inventory.HealthMark{At: at, Revision: h.revisionOf(id)}
	h.writer.addHealthy(id, mark)
}

// revisionOf is the revision of a workload, or its template hash, or "".
func (h *historyRecorder) revisionOf(id inventory.EntityID) string {
	entity, ok := h.model.Entity(id)
	if !ok {
		return ""
	}
	for _, name := range []string{kube.AttrRevision, kube.AttrTemplateHash} {
		if attribute, ok := entity.Attribute(name); ok {
			if text := attribute.Value.AsText(); text != "" {
				return text
			}
		}
	}
	return ""
}
