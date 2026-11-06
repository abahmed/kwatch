package release

import (
	"math"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// startRun is how long a run may last and still have stopped at start,
// as the startup error rule reads it (explain.FatalStartLine's caller).
const startRun = 5 * time.Minute

// detailOf compares the workload before and after the release r: its
// usual restarts and ready time from its baseline, the restarts of the
// pods the release created, and the first fatal line one of their
// containers stopped with soon after it started.
func detailOf(
	model inventory.HistoryReader, id inventory.EntityID, r Record,
	now time.Time,
) *Detail {
	d := &Detail{}
	if base := model.Baselines(); base != nil {
		d.RestartsBefore = usual(base, id, inventory.MetricRestartsPerHour)
		d.ReadyBefore = usual(base, id, inventory.MetricReadySeconds)
	}
	restarts, created := 0.0, false
	for _, pod := range kube.OwnedPods(model, id) {
		p, ok := model.Entity(pod)
		if !ok || p.FirstSeen.Before(r.At) {
			continue
		}
		created = true
		for _, c := range model.Related(pod, inventory.OwnedBy,
			inventory.Incoming) {
			ce, ok := model.Entity(c)
			if !ok || c.Kind != kube.KindContainer {
				continue
			}
			restarts += float64(count(ce, kube.AttrRestarts))
			if d.ErrorLine == "" {
				d.ErrorLine = startError(ce)
			}
		}
	}
	if created {
		hours := max(now.Sub(r.At).Hours(), 1.0/60)
		// One decimal: the rate moves with every look otherwise.
		rate := math.Round(restarts/hours*10) / 10
		d.RestartsAfter = &rate
	}
	if *d == (Detail{}) {
		return nil
	}
	return d
}

// usual is the workload's median of metric, or nil without history.
func usual(
	base *inventory.Baselines, id inventory.EntityID, m inventory.Metric,
) *float64 {
	stat, ok := base.Stat(id, m)
	if !ok || stat.Count < inventory.MinBaselineSamples {
		return nil
	}
	median := stat.Quantile(0.5)
	return &median
}

// startError is the fatal line a container's last run stopped with, when
// that run ended within startRun of its start.
func startError(c inventory.Entity) string {
	line := ""
	if a, ok := c.Attribute(kube.AttrLastErrorLine); ok {
		line = a.Value.AsText()
	}
	if !explain.FatalStartLine(line) {
		return ""
	}
	started, ok1 := c.Attribute(kube.AttrLastStarted)
	finished, ok2 := c.Attribute(kube.AttrLastFinished)
	if ok1 && ok2 {
		begin, end := started.Value.AsTime(), finished.Value.AsTime()
		if !begin.IsZero() && !end.IsZero() &&
			end.Sub(begin) >= startRun {
			return ""
		}
	}
	return line
}

func sameDetail(a, b *Detail) bool {
	if a == nil || b == nil {
		return a == b
	}
	return samePtr(a.RestartsBefore, b.RestartsBefore) &&
		samePtr(a.RestartsAfter, b.RestartsAfter) &&
		samePtr(a.ReadyBefore, b.ReadyBefore) && a.ErrorLine == b.ErrorLine
}

func samePtr(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
