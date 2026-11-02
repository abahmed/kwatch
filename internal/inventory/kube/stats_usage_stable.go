package kube

import (
	"slices"
	"time"
)

// usageRefreshEvery is how long a stored record may keep the readings of
// a steady container. The store rewrites only records whose bytes
// changed, and a running container's sample count, last two readings and
// times move on every poll: storing them as they are rewrote every live
// container's record on every save (some 5,000 records, 10 MiB, every 5
// minutes). A record whose lasting content did not change is handed out
// as stored until it is this old.
const usageRefreshEvery = time.Hour

// stableRecord is record(now), unless the record stored before differs
// only in what moves on every poll (Seen and the current run's count,
// last readings and their times) and is younger than usageRefreshEvery:
// then it is that stored record, unchanged. Its moving fields are all
// from one moment, so a restored run still pairs its last two readings
// with their own times. The caller holds the log's lock.
func (h *memoryHistory) stableRecord(now time.Time) UsageRecord {
	fresh := h.record(now)
	if h.stored != nil && now.Sub(h.stored.Seen) < usageRefreshEvery &&
		sameLasting(*h.stored, fresh) {
		return *h.stored
	}
	h.stored = &fresh
	return fresh
}

// sameLasting reports whether two records hold the same lasting content:
// everything but what moves on every poll.
func sameLasting(a, b UsageRecord) bool {
	return a.Node == b.Node && a.Started.Equal(b.Started) &&
		sameRunStart(a.Run, b.Run) && sameRun(a.Prev, b.Prev) &&
		slices.EqualFunc(a.Hours, b.Hours, sameHour)
}

// sameRunStart compares what a current run keeps: its start, peak and
// drawdown. The count, last readings and their times move every poll.
func sameRunStart(a, b *UsageRun) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.FirstAt.Equal(b.FirstAt) && a.First == b.First &&
		a.Peak == b.Peak && a.Drawdown == b.Drawdown
}

// sameRun compares an ended run in full; it no longer moves.
func sameRun(a, b *UsageRun) bool {
	if a == nil || b == nil {
		return a == b
	}
	return sameRunStart(a, b) && a.Samples == b.Samples &&
		a.LastAt.Equal(b.LastAt) && a.BeforeAt.Equal(b.BeforeAt) &&
		a.Last == b.Last && a.Before == b.Before
}

func sameHour(a, b UsageHour) bool {
	return a.Start.Equal(b.Start) && a.MemoryBytes == b.MemoryBytes &&
		a.CPUMilli == b.CPUMilli
}
