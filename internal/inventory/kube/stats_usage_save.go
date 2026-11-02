package kube

import (
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/inventory"
)

// usageSaveEvery spaces out saves of the usage history. The history is
// hourly peaks, so losing a few minutes of it to a crash changes little.
const usageSaveEvery = 5 * time.Minute

// UsageStore keeps the memory and CPU history of containers across
// restarts. SaveUsage replaces everything stored with records: a
// container missing from it is deleted.
type UsageStore interface {
	LoadUsage() (map[inventory.EntityID]UsageRecord, error)
	SaveUsage(records map[inventory.EntityID]UsageRecord) error
}

// loadUsage restores the stored history. When it cannot be read, the
// history of this session is not saved either, so a read failure never
// replaces a good stored history with an empty one.
func (p *StatsPoller) loadUsage() {
	if p.cfg.History == nil {
		return
	}
	records, err := p.cfg.History.LoadUsage()
	if err != nil {
		klog.ErrorS(err, "usage history not restored and not saved",
			"component", "kubelet-stats", "operation", "load-usage")
		return
	}
	p.saving = true
	restored := p.memory.restore(records, p.cfg.Now())
	klog.V(2).InfoS("usage history restored",
		"component", "kubelet-stats", "containers", restored)
}

// saveUsage stores the history at most every usageSaveEvery, and always
// when final. A failed save is logged and retried at the next turn: the
// spacing counts from the last save that worked. The store writes only
// records that changed and is fenced by the state file's epoch, so a
// replica that lost the state lock cannot overwrite the new leader's.
func (p *StatsPoller) saveUsage(final bool) {
	if !p.saving {
		return
	}
	now := p.cfg.Now()
	if !final && !p.saved.IsZero() && now.Sub(p.saved) < usageSaveEvery {
		return
	}
	if err := p.cfg.History.SaveUsage(p.memory.snapshot(now)); err != nil {
		klog.ErrorS(err, "usage history not saved",
			"component", "kubelet-stats", "operation", "save-usage")
		return
	}
	p.saved = now
}
