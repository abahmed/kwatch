package kube

import (
	"context"
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// Memory history written to a container beside its current use, so that
// when it is OOM-killed (and its use starts over) a detector can still
// say what it used before. All of it describes the past.
const (
	// AttrMemoryPeak24h is the highest working set seen in the last 24
	// hours, across restarts.
	AttrMemoryPeak24h = "memory.peak.24h.bytes"
	// The memory.prev attributes describe the container's previous run:
	// the one that ended when the container last restarted.
	AttrMemoryPrevStart    = "memory.prev.start.bytes"
	AttrMemoryPrevPeak     = "memory.prev.peak.bytes"
	AttrMemoryPrevEnded    = "memory.prev.ended"
	AttrMemoryPrevSeconds  = "memory.prev.seconds"
	AttrMemoryPrevDrawdown = "memory.prev.drawdown.pct"

	// memoryWindowHours is how far back the peak looks. One slot per
	// hour keeps the history small for a whole cluster of containers.
	memoryWindowHours = 24
)

// memoryRun is what kwatch saw of one run of a container, between its
// start and its end (or now).
type memoryRun struct {
	samples         int
	firstAt, lastAt time.Time
	first, peak     float64
	// drawdown is the largest fall, as a fraction, from the highest use
	// so far: near zero for a steady climb, large for a saw-tooth.
	drawdown float64
}

func (r *memoryRun) add(at time.Time, bytes float64) {
	if r.samples == 0 {
		r.firstAt, r.first = at, bytes
	}
	if bytes >= r.peak {
		r.peak = bytes
	} else {
		r.drawdown = max(r.drawdown, (r.peak-bytes)/r.peak)
	}
	r.samples++
	r.lastAt = at
}

// hourPeak is the highest working set of one clock hour.
type hourPeak struct {
	hour  int64
	bytes float64
}

// memoryHistory is one container's peak by hour and its last two runs.
type memoryHistory struct {
	// node is where the container was last read.
	node    string
	started time.Time
	run     memoryRun
	prev    memoryRun
	hours   [memoryWindowHours]hourPeak
}

func (h *memoryHistory) observe(
	started, at time.Time, bytes float64,
) {
	// A new start time is a new run: the container restarted.
	if h.run.samples > 0 && !started.IsZero() && !started.Equal(h.started) {
		h.prev, h.run = h.run, memoryRun{}
	}
	h.started = started
	h.run.add(at, bytes)
	hour := at.Unix() / 3600
	slot := &h.hours[hour%memoryWindowHours]
	if slot.hour != hour {
		*slot = hourPeak{hour: hour}
	}
	slot.bytes = max(slot.bytes, bytes)
}

func (h *memoryHistory) peak24h(now time.Time) float64 {
	oldest := now.Unix()/3600 - memoryWindowHours
	peak := 0.0
	for _, slot := range h.hours {
		if slot.hour > oldest {
			peak = max(peak, slot.bytes)
		}
	}
	return peak
}

// attributes are the history as container attributes.
func (h *memoryHistory) attributes(
	now time.Time,
) map[string]inventory.Value {
	attrs := map[string]inventory.Value{
		AttrMemoryPeak24h: inventory.Number(h.peak24h(now)),
	}
	if h.prev.samples == 0 {
		return attrs
	}
	attrs[AttrMemoryPrevStart] = inventory.Number(h.prev.first)
	attrs[AttrMemoryPrevPeak] = inventory.Number(h.prev.peak)
	attrs[AttrMemoryPrevEnded] = inventory.Time(h.prev.lastAt)
	attrs[AttrMemoryPrevSeconds] = inventory.Number(
		h.prev.lastAt.Sub(h.prev.firstAt).Seconds())
	attrs[AttrMemoryPrevDrawdown] = inventory.Number(h.prev.drawdown * 100)
	return attrs
}

// memoryLog keeps the memory history of every container the kubelets
// report.
//
// The history is published under its own source (memorySource), apart from
// the live reading. A container that is crashing or waiting leaves the
// kubelet summary, and its live reading is cleared with it; the history is
// exactly what an OOM explanation needs then, so it is published again on
// every poll of the node the container ran on, until the 24 hour window
// has passed or the container leaves the model.
type memoryLog struct {
	mu         sync.Mutex
	containers map[inventory.EntityID]*memoryHistory
}

func newMemoryLog() *memoryLog {
	return &memoryLog{containers: map[inventory.EntityID]*memoryHistory{}}
}

// note records one reading of a container on node. started is the
// kubelet's start time of the container's current run; zero when it did
// not say.
func (l *memoryLog) note(
	id inventory.EntityID, node string, started, at time.Time, bytes float64,
) {
	l.mu.Lock()
	defer l.mu.Unlock()
	history := l.containers[id]
	if history == nil {
		history = &memoryHistory{}
		l.containers[id] = history
	}
	history.node = node
	history.observe(started, at, bytes)
}

// observations publishes the history of every container last read on
// node, including those the latest summary no longer lists.
func (l *memoryLog) observations(
	node string, now time.Time,
) []inventory.Observation {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []inventory.Observation
	for id, history := range l.containers {
		if history.node == node {
			out = append(out, inventory.Observation{
				Kind: inventory.Observed, Source: memorySource, At: now,
				Entity: id, Attributes: history.attributes(now),
			})
		}
	}
	return out
}

// forgetBefore drops containers not read since cutoff, and containers
// that live is set for and does not list, and returns them, so their
// history can be removed from the model too. A nil live keeps every
// container the window allows.
func (l *memoryLog) forgetBefore(
	cutoff time.Time, live map[inventory.EntityID]bool,
) []inventory.EntityID {
	l.mu.Lock()
	defer l.mu.Unlock()
	var dropped []inventory.EntityID
	for id, history := range l.containers {
		gone := live != nil && !live[id]
		if gone || history.run.lastAt.Before(cutoff) {
			delete(l.containers, id)
			dropped = append(dropped, id)
		}
	}
	return dropped
}

// clearMemory forgets containers not read for the whole window, or gone
// from the model, and removes their history from the model.
func (p *StatsPoller) clearMemory(ctx context.Context, now time.Time) {
	var live map[inventory.EntityID]bool
	if p.cfg.Containers != nil {
		live = map[inventory.EntityID]bool{}
		for _, id := range p.cfg.Containers() {
			live[id] = true
		}
	}
	dropped := p.memory.forgetBefore(
		now.Add(-memoryWindowHours*time.Hour), live)
	var cleared []inventory.Observation
	for _, id := range dropped {
		cleared = append(cleared, emptyReading(
			published{entity: id, source: memorySource}, now))
	}
	if len(cleared) > 0 {
		p.cfg.Submit(ctx, cleared...)
	}
}
