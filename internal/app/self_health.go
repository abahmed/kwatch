package app

import (
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// selfHealthEvery is how often the model's sizes are logged. A slow
// climb in any of them shows up here long before the container's memory
// limit does. It matches the stats poller's self-health line.
const selfHealthEvery = 30 * time.Minute

// modelHealthLog writes the model's sizes when a line is due.
type modelHealthLog struct {
	last time.Time
}

// log writes the line at most once per selfHealthEvery; the first call
// is due, so a restart logs its starting point.
func (l *modelHealthLog) log(model *inventory.Model, now time.Time) {
	if !l.last.IsZero() && now.Sub(l.last) < selfHealthEvery {
		return
	}
	l.last = now
	klog.InfoS("self-health", append([]any{
		"component", "self-health-model"}, modelHealthFields(model)...)...)
}

// modelHealthFields are the model's sizes and the runtime numbers, as
// key/value pairs for the self-health line.
func modelHealthFields(model *inventory.Model) []any {
	stats := model.Stats()
	return append(kube.RuntimeFields(),
		"entities", stats.Entities, "tombstones", stats.Tombstones,
		"relations", stats.Relations, "changes", stats.Changes,
		"notes", stats.Notes, "noteParts", stats.NoteParts,
		"healthMarks", stats.Health, "recentChanges", stats.Recent,
		"churnChanges", stats.Churn, "baselines", stats.Baselines)
}
