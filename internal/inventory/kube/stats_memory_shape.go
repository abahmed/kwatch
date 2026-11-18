package kube

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// The shape of a container's memory history, for an OOM kill: how much
// history there is, how often it was near its peak, and where its last
// runs ended. All of it describes the past.
const (
	// AttrMemoryHistorySince is the start of the oldest clock hour of
	// the 24 hour window that has a reading: the history kwatch really
	// has, which is less than 24 hours for a young container or a young
	// kwatch.
	AttrMemoryHistorySince = "memory.history.since"
	// AttrMemoryPeakHours counts the hours of the window whose highest
	// reading was within peakHourShare of the window's peak.
	AttrMemoryPeakHours = "memory.peak.hours"
	// AttrMemoryRunsEnded counts the ended runs of the window whose
	// peak is known (at most maxRunEnds), and AttrMemoryRunsLow and
	// AttrMemoryRunsHigh are the lowest and highest of those peaks.
	AttrMemoryRunsEnded = "memory.runs.ended"
	AttrMemoryRunsLow   = "memory.runs.low.bytes"
	AttrMemoryRunsHigh  = "memory.runs.high.bytes"

	// peakHourShare is how close to the peak an hour must come to
	// count as a peak hour.
	peakHourShare = 0.95
	// maxRunEnds is how many ended runs the history keeps.
	maxRunEnds = 5
)

// runEnd is the peak of one ended run, and when it ended.
type runEnd struct {
	at   time.Time
	peak float64
}

// endRun records the run that just ended, keeping the last maxRunEnds.
func (h *memoryHistory) endRun(r memoryRun) {
	if r.samples == 0 || r.peak <= 0 {
		return
	}
	h.ends = append(h.ends, runEnd{at: r.lastAt, peak: r.peak})
	if len(h.ends) > maxRunEnds {
		h.ends = h.ends[len(h.ends)-maxRunEnds:]
	}
}

// addShape writes the shape attributes of the window ending now.
func (h *memoryHistory) addShape(
	attrs map[string]inventory.Value, now time.Time, peak float64,
) {
	hours := h.usageHours(now)
	if len(hours) == 0 {
		return
	}
	attrs[AttrMemoryHistorySince] = inventory.Time(hours[0].Start)
	if peak > 0 {
		count := 0
		for _, hour := range hours {
			if hour.MemoryBytes >= peakHourShare*peak {
				count++
			}
		}
		attrs[AttrMemoryPeakHours] = inventory.Number(float64(count))
	}
	h.addRunEnds(attrs, now)
}

// addRunEnds writes how many runs of the window ended and between which
// peaks.
func (h *memoryHistory) addRunEnds(
	attrs map[string]inventory.Value, now time.Time,
) {
	cutoff := now.Add(-memoryWindowHours * time.Hour)
	count, low, high := 0, 0.0, 0.0
	for _, end := range h.ends {
		if end.at.Before(cutoff) {
			continue
		}
		if count == 0 || end.peak < low {
			low = end.peak
		}
		high = max(high, end.peak)
		count++
	}
	if count == 0 {
		return
	}
	attrs[AttrMemoryRunsEnded] = inventory.Number(float64(count))
	attrs[AttrMemoryRunsLow] = inventory.Number(low)
	attrs[AttrMemoryRunsHigh] = inventory.Number(high)
}

// UsageEnd is the stored peak of one ended run (runEnd).
type UsageEnd struct {
	At   time.Time `json:"at"`
	Peak float64   `json:"peak"`
}

func endRecords(ends []runEnd) []UsageEnd {
	out := make([]UsageEnd, 0, len(ends))
	for _, e := range ends {
		out = append(out, UsageEnd{At: e.at, Peak: e.peak})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func endsOf(records []UsageEnd) []runEnd {
	var out []runEnd
	for _, r := range records {
		if peak := usable(r.Peak); peak > 0 && !r.At.IsZero() {
			out = append(out, runEnd{at: r.At, peak: peak})
		}
	}
	if len(out) > maxRunEnds {
		out = out[len(out)-maxRunEnds:]
	}
	return out
}
