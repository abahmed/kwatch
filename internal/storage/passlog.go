package storage

import (
	"time"

	"k8s.io/klog/v2"
)

// logPass says what one compactor pass did and what the state file looks
// like afterwards, so the size trend can be watched in the log: the file,
// the free pages inside it, and the live bytes of the big buckets.
func logPass(r PassResult, sizes map[Bucket]int64, took time.Duration) {
	klog.InfoS("state compaction pass", "component", "state",
		"operation", "compact", "expired", r.Expired, "evicted", r.Evicted,
		"fileBytes", r.FileBytes, "freeBytes", r.FreeBytes,
		"logicalBytes", r.Bytes, "pinnedBytes", r.PinnedBytes,
		"timelineBytes", sizes[Timeline], "evidenceBytes", sizes[Evidence],
		"incidentBytes", sizes[Incidents], "overCap", r.OverCap,
		"rewrote", r.Rewrote, "durationMs", took.Milliseconds())
}
