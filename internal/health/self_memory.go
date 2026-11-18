package health

import (
	"math"
	"runtime/debug"
	"runtime/metrics"
	"strconv"
	"strings"
	"time"
)

// Memory levels of kwatch's own memory (ResourceUse.MemoryLevel).
const (
	MemoryOK   = ""
	MemoryWarn = "warn"
	MemoryBad  = "bad"
)

// Memory alarm thresholds, as a share of the memory base (memoryBase):
// the Go memory limit when one is set, else the container limit. The
// Go runtime collects harder as it nears its limit, so 80% of it is the
// first sign that kwatch needs more room and 95% means the collector
// runs nearly all the time. A level is left only well below the line
// that raised it (70% and 90%), and any change must hold for
// memoryHold, so a value hovering near a line does not flap.
const (
	memoryWarnEnter = 0.80
	memoryWarnExit  = 0.70
	memoryBadEnter  = 0.95
	memoryBadExit   = 0.90
	memoryHold      = 3 * time.Minute
)

// goMemory is the memory the Go runtime holds and has not returned to
// the system, the figure its memory limit counts, and that limit (0
// when none is set). Unlike the container's memory use it leaves out
// file pages (the mapped state file) the system can take back.
func goMemory() (used, limit int64) {
	samples := []metrics.Sample{
		{Name: "/memory/classes/total:bytes"},
		{Name: "/memory/classes/heap/released:bytes"},
	}
	metrics.Read(samples)
	total, released := sampleBytes(samples[0]), sampleBytes(samples[1])
	if total > released {
		used = total - released
	}
	if l := debug.SetMemoryLimit(-1); l > 0 && l < math.MaxInt64 {
		limit = l
	}
	return used, limit
}

func sampleBytes(s metrics.Sample) int64 {
	if s.Value.Kind() != metrics.KindUint64 {
		return 0
	}
	v := s.Value.Uint64()
	if v > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(v)
}

// memoryBase is what the alarm compares the Go memory with: the Go
// memory limit when it is set below the container limit, else the
// container limit; 0 when neither is set.
func memoryBase(goLimit, containerLimit int64) int64 {
	if goLimit > 0 && (containerLimit <= 0 || goLimit < containerLimit) {
		return goLimit
	}
	return containerLimit
}

// fileCache is the container's file page cache from memory.stat (v2
// "file", v1 "total_cache" or "cache"): memory the system can free.
func (r *UsageReader) fileCache() (int64, bool) {
	for _, f := range []struct{ file, key string }{
		{"memory.stat", "file"},
		{"memory/memory.stat", "total_cache"},
		{"memory/memory.stat", "cache"},
	} {
		text, ok := r.text(f.file)
		if !ok {
			continue
		}
		if n, found := statField(text, f.key); found {
			return n, true
		}
	}
	return 0, false
}

// statField reads "key value" from a memory.stat body.
func statField(text, key string) (int64, bool) {
	for _, line := range strings.Split(text, "\n") {
		f := strings.Fields(line)
		if len(f) != 2 || f[0] != key {
			continue
		}
		n, err := strconv.ParseInt(f[1], 10, 64)
		return n, err == nil && n >= 0
	}
	return 0, false
}

// memoryAlarm turns memory samples into a level with hysteresis: the
// level changes only once the new one held for memoryHold.
type memoryAlarm struct {
	level string
	// waiting is set while pending, a level other than level, has
	// been called for since since.
	waiting bool
	pending string
	since   time.Time
}

// observe takes one sample, used of base at now, and returns the
// level. Without a base nothing is judged.
func (a *memoryAlarm) observe(now time.Time, used, base int64) string {
	target := MemoryOK
	if base > 0 {
		target = memoryTarget(a.level, float64(used)/float64(base))
	}
	switch {
	case target == a.level:
		a.waiting = false
	case !a.waiting || target != a.pending:
		a.waiting, a.pending, a.since = true, target, now
	case now.Sub(a.since) >= memoryHold:
		a.level, a.waiting = target, false
	}
	return a.level
}

// memoryTarget is the level a share calls for, given the current one:
// a level is kept until the share falls below its exit line.
func memoryTarget(current string, share float64) string {
	switch {
	case share >= memoryBadEnter,
		current == MemoryBad && share >= memoryBadExit:
		return MemoryBad
	case share >= memoryWarnEnter,
		current != MemoryOK && share >= memoryWarnExit:
		return MemoryWarn
	}
	return MemoryOK
}
