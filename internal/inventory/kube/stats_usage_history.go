package kube

import (
	"math"
	"sort"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// maxUsageRecords bounds the usage history kept across a restart: the
// containers read most recently win. A record is about 2 KiB, so the
// stored history stays near 10 MiB however large the cluster.
const maxUsageRecords = 5000

// UsageRecord is the stored memory and CPU history of one container.
// It is part of the state file: fields may be added, never renamed, and
// a record that misses one restores the rest.
type UsageRecord struct {
	// Node is where the container was last read, Seen when.
	Node string    `json:"node,omitempty"`
	Seen time.Time `json:"seen"`
	// Started is the start time of the container's current run.
	Started time.Time `json:"started,omitzero"`
	// Run and Prev are the current and previous runs' memory use.
	Run  *UsageRun `json:"run,omitempty"`
	Prev *UsageRun `json:"prev,omitempty"`
	// Hours are the hourly peaks of the last 24 hours, oldest first.
	Hours []UsageHour `json:"hours,omitempty"`
}

// UsageRun is the memory use of one run of a container (memoryRun).
type UsageRun struct {
	Samples  int       `json:"samples"`
	FirstAt  time.Time `json:"firstAt"`
	LastAt   time.Time `json:"lastAt"`
	BeforeAt time.Time `json:"beforeAt,omitzero"`
	First    float64   `json:"first"`
	Peak     float64   `json:"peak"`
	Last     float64   `json:"last"`
	Before   float64   `json:"before,omitempty"`
	Drawdown float64   `json:"drawdown,omitempty"`
}

// UsageHour is the highest working set and CPU use of one clock hour.
type UsageHour struct {
	// Start is the start of the hour, in UTC.
	Start       time.Time `json:"start"`
	MemoryBytes float64   `json:"memoryBytes,omitempty"`
	CPUMilli    float64   `json:"cpuMilli,omitempty"`
}

// UsageHours returns the hourly memory and CPU peaks of a container over
// the last 24 hours, oldest first; nil when it has no history. The
// dashboard reads it to draw a container's day.
func (p *StatsPoller) UsageHours(id inventory.EntityID) []UsageHour {
	now := p.cfg.Now()
	p.memory.mu.Lock()
	defer p.memory.mu.Unlock()
	history := p.memory.containers[id]
	if history == nil {
		return nil
	}
	return history.usageHours(now)
}

// usageHours lists the slots inside the window, oldest first.
func (h *memoryHistory) usageHours(now time.Time) []UsageHour {
	oldest := now.Unix()/3600 - memoryWindowHours
	var out []UsageHour
	for _, slot := range h.hours {
		if slot.hour > oldest && slot.hour != 0 {
			out = append(out, UsageHour{
				Start:       time.Unix(slot.hour*3600, 0).UTC(),
				MemoryBytes: slot.bytes, CPUMilli: slot.milli,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Start.Before(out[j].Start)
	})
	return out
}

// snapshot is the history to store: every container read within the
// window, at most maxUsageRecords of them, the most recently read first.
func (l *memoryLog) snapshot(now time.Time) map[inventory.EntityID]UsageRecord {
	cutoff := now.Add(-memoryWindowHours * time.Hour)
	l.mu.Lock()
	defer l.mu.Unlock()
	ids := make([]inventory.EntityID, 0, len(l.containers))
	for id, history := range l.containers {
		if !history.seen.Before(cutoff) {
			ids = append(ids, id)
		}
	}
	ids = newestFirst(ids, func(id inventory.EntityID) time.Time {
		return l.containers[id].seen
	})
	out := make(map[inventory.EntityID]UsageRecord, len(ids))
	for _, id := range ids {
		out[id] = l.containers[id].stableRecord(now)
	}
	return out
}

// restore adds stored histories of containers the log does not hold
// yet. Records outside the window or with no reading are skipped, and
// at most maxUsageRecords are kept, the most recently read first.
func (l *memoryLog) restore(
	records map[inventory.EntityID]UsageRecord, now time.Time,
) int {
	cutoff := now.Add(-memoryWindowHours * time.Hour)
	ids := make([]inventory.EntityID, 0, len(records))
	for id, r := range records {
		if !id.IsZero() && !r.Seen.Before(cutoff) && !r.Seen.After(now) {
			ids = append(ids, id)
		}
	}
	ids = newestFirst(ids, func(id inventory.EntityID) time.Time {
		return records[id].Seen
	})
	l.mu.Lock()
	defer l.mu.Unlock()
	restored := 0
	for _, id := range ids {
		if l.containers[id] == nil {
			l.containers[id] = historyOf(records[id], now)
			restored++
		}
	}
	return restored
}

// newestFirst sorts ids by seen, newest first, and keeps at most
// maxUsageRecords.
func newestFirst(
	ids []inventory.EntityID, seen func(inventory.EntityID) time.Time,
) []inventory.EntityID {
	sort.Slice(ids, func(i, j int) bool {
		a, b := seen(ids[i]), seen(ids[j])
		if !a.Equal(b) {
			return a.After(b)
		}
		return ids[i].String() < ids[j].String()
	})
	return ids[:min(len(ids), maxUsageRecords)]
}

// record is the history as stored.
func (h *memoryHistory) record(now time.Time) UsageRecord {
	// Seen and the last reading of a run are stored to the save
	// interval: they move every poll, and exact times would make the
	// store rewrite every record on every save.
	return UsageRecord{
		Node: h.node, Seen: h.seen.Truncate(usageSaveEvery),
		Started: h.started,
		Run:     runRecord(h.run), Prev: runRecord(h.prev),
		Hours: h.usageHours(now),
	}
}

func runRecord(r memoryRun) *UsageRun {
	if r.samples == 0 {
		return nil
	}
	return &UsageRun{
		Samples: r.samples, FirstAt: r.firstAt,
		LastAt:   r.lastAt.Truncate(usageSaveEvery),
		BeforeAt: r.beforeAt, First: r.first, Peak: r.peak,
		Last: r.last, Before: r.before, Drawdown: r.drawdown,
	}
}

// historyOf rebuilds a history from its record. Hours outside the
// window and readings that are not a usable number are dropped.
func historyOf(r UsageRecord, now time.Time) *memoryHistory {
	h := &memoryHistory{
		node: r.Node, seen: r.Seen, started: r.Started,
		run: runOf(r.Run), prev: runOf(r.Prev),
	}
	oldest := now.Unix()/3600 - memoryWindowHours
	newest := now.Unix() / 3600
	for _, hour := range r.Hours {
		at := hour.Start.Unix() / 3600
		if at <= oldest || at > newest {
			continue
		}
		slot := h.slot(hour.Start)
		slot.bytes = max(slot.bytes, usable(hour.MemoryBytes))
		slot.milli = max(slot.milli, usable(hour.CPUMilli))
	}
	return h
}

func runOf(r *UsageRun) memoryRun {
	if r == nil || r.Samples <= 0 {
		return memoryRun{}
	}
	return memoryRun{
		samples: r.Samples, firstAt: r.FirstAt, lastAt: r.LastAt,
		beforeAt: r.BeforeAt, first: usable(r.First),
		peak: usable(r.Peak), last: usable(r.Last),
		before: usable(r.Before), drawdown: usable(r.Drawdown),
	}
}

// usable maps a negative or non-finite stored number to zero.
func usable(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return 0
	}
	return v
}
