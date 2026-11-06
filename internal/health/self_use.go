package health

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// CgroupRoot is where a container sees its own cgroup.
const CgroupRoot = "/sys/fs/cgroup"

// SelfUseEvery is how often the CPU use is sampled.
const SelfUseEvery = 15 * time.Second

// noMemoryLimit is the smallest cgroup v1 memory limit that means
// "none": the kernel reports a page-rounded maximum there.
const noMemoryLimit = int64(1) << 60

// ResourceUse is kwatch's own memory, CPU and disk use against its
// container's limits. A limit of 0 means none is set; a Known field
// false means that resource can't be read (no cgroup, not Linux).
type ResourceUse struct {
	MemoryKnown bool
	MemoryBytes int64
	MemoryLimit int64
	CPUKnown    bool
	CPUCores    float64
	CPULimit    float64
	DiskKnown   bool
	DiskUsed    int64
	DiskTotal   int64
}

// UsageReader reads kwatch's own use from its cgroup (v2, else v1)
// and the volume of its state directory. CPU use is a rate, so it
// comes from two samples; Run takes them.
type UsageReader struct {
	root, dir string
	now       func() time.Time

	mu       sync.Mutex
	lastUse  time.Duration
	lastAt   time.Time
	cores    float64
	hasCores bool
}

// NewUsageReader reads the cgroup under root and the volume holding
// dir; an empty dir leaves the disk unknown.
func NewUsageReader(
	root, dir string, now func() time.Time,
) *UsageReader {
	return &UsageReader{root: root, dir: dir, now: now}
}

// Run samples the CPU use now and on each tick until ctx ends.
func (r *UsageReader) Run(ctx context.Context, tick <-chan time.Time) {
	r.Sample()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick:
			r.Sample()
		}
	}
}

// Sample reads the CPU time used so far and, from the previous
// sample, the cores used in between.
func (r *UsageReader) Sample() {
	used, ok := r.cpuUsed()
	at := r.now()
	r.mu.Lock()
	defer r.mu.Unlock()
	if !ok {
		r.lastAt, r.hasCores = time.Time{}, false
		return
	}
	if !r.lastAt.IsZero() && at.After(r.lastAt) && used >= r.lastUse {
		r.cores = float64(used-r.lastUse) / float64(at.Sub(r.lastAt))
		r.hasCores = true
	}
	r.lastUse, r.lastAt = used, at
}

// Use reads the memory and disk now, with the last CPU rate.
func (r *UsageReader) Use() ResourceUse {
	var u ResourceUse
	u.MemoryBytes, u.MemoryLimit, u.MemoryKnown = r.memory()
	r.mu.Lock()
	u.CPUCores, u.CPUKnown = r.cores, r.hasCores
	r.mu.Unlock()
	if u.CPUKnown {
		u.CPULimit = r.cpuLimit()
	}
	if r.dir != "" {
		u.DiskUsed, u.DiskTotal, u.DiskKnown = diskUse(r.dir)
	}
	return u
}

func (r *UsageReader) memory() (used, limit int64, ok bool) {
	if used, ok = r.number("memory.current"); ok {
		limit, _ = r.number("memory.max")
		return used, limit, true
	}
	if used, ok = r.number("memory/memory.usage_in_bytes"); !ok {
		return 0, 0, false
	}
	limit, _ = r.number("memory/memory.limit_in_bytes")
	if limit >= noMemoryLimit {
		limit = 0
	}
	return used, limit, true
}

func (r *UsageReader) cpuUsed() (time.Duration, bool) {
	if s, ok := r.text("cpu.stat"); ok {
		for _, line := range strings.Split(s, "\n") {
			f := strings.Fields(line)
			if len(f) == 2 && f[0] == "usage_usec" {
				us, err := strconv.ParseInt(f[1], 10, 64)
				return time.Duration(us) * time.Microsecond, err == nil
			}
		}
		return 0, false
	}
	for _, p := range []string{"cpuacct/cpuacct.usage",
		"cpu,cpuacct/cpuacct.usage"} {
		if ns, ok := r.number(p); ok {
			return time.Duration(ns), true
		}
	}
	return 0, false
}

// cpuLimit is the cores the container may use, 0 when unlimited.
func (r *UsageReader) cpuLimit() float64 {
	if s, ok := r.text("cpu.max"); ok {
		f := strings.Fields(s)
		if len(f) != 2 || f[0] == "max" {
			return 0
		}
		return ratio(f[0], f[1])
	}
	for _, dir := range []string{"cpu", "cpu,cpuacct"} {
		q, okQ := r.text(dir + "/cpu.cfs_quota_us")
		p, okP := r.text(dir + "/cpu.cfs_period_us")
		if okQ && okP {
			return ratio(q, p)
		}
	}
	return 0
}

func ratio(quota, period string) float64 {
	q, err1 := strconv.ParseFloat(strings.TrimSpace(quota), 64)
	p, err2 := strconv.ParseFloat(strings.TrimSpace(period), 64)
	if err1 != nil || err2 != nil || q <= 0 || p <= 0 {
		return 0
	}
	return q / p
}

// number reads a file holding one integer; "max" reads as 0 (none).
func (r *UsageReader) number(name string) (int64, bool) {
	s, ok := r.text(name)
	if !ok {
		return 0, false
	}
	if s == "max" {
		return 0, true
	}
	n, err := strconv.ParseInt(s, 10, 64)
	return n, err == nil && n >= 0
}

func (r *UsageReader) text(name string) (string, bool) {
	b, err := os.ReadFile(filepath.Join(r.root, name))
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(b)), true
}
