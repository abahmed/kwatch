package storage

import (
	"time"

	"k8s.io/klog/v2"
)

// logPass says what one compactor pass did and what the state file looks
// like afterwards, so the size trend can be watched in the log: the file,
// the free pages inside it, the live bytes of every bucket and how far
// back the timeline reaches.
func logPass(r PassResult, sizes map[Bucket]int64, took time.Duration) {
	fields := []any{"component", "state",
		"operation", "compact", "expired", r.Expired, "evicted", r.Evicted,
		"fileBytes", r.FileBytes, "freeBytes", r.FreeBytes,
		"logicalBytes", r.Bytes, "pinnedBytes", r.PinnedBytes,
		"overCap", r.OverCap, "rewrote", r.Rewrote,
		"oldestTimelineAge", r.OldestTimeline.Round(time.Minute).String(),
		"durationMs", took.Milliseconds()}
	fields = append(fields, bucketFields(sizes)...)
	klog.InfoS("state compaction pass", fields...)
}

// bucketFields is one "<bucket>Bytes" pair per bucket, in spec order. The
// sizes come from the scan the pass did anyway, so this costs nothing.
func bucketFields(sizes map[Bucket]int64) []any {
	out := make([]any, 0, 2*len(specs))
	for _, sp := range specs {
		out = append(out, string(sp.name)+"Bytes", sizes[sp.name])
	}
	return out
}
