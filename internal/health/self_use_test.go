package health

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeCgroup(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// steps returns a clock that moves on by step on every read.
func steps(step time.Duration) func() time.Time {
	at := time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)
	return func() time.Time {
		at = at.Add(step)
		return at
	}
}

func TestUsageReaderCgroupV2(t *testing.T) {
	root := writeCgroup(t, map[string]string{
		"memory.current": "190840832",
		"memory.max":     "268435456",
		"cpu.max":        "50000 100000",
		"cpu.stat":       "usage_usec 1000000\nuser_usec 1",
	})
	r := NewUsageReader(root, "", steps(10*time.Second))
	r.Sample()
	if u := r.Use(); u.CPUKnown {
		t.Fatalf("one sample gave a CPU rate: %+v", u)
	}
	stat := "usage_usec 1400000\nuser_usec 1"
	if err := os.WriteFile(filepath.Join(root, "cpu.stat"),
		[]byte(stat), 0o600); err != nil {
		t.Fatal(err)
	}
	r.Sample()
	u := r.Use()
	if !u.MemoryKnown || u.MemoryBytes != 190840832 ||
		u.MemoryLimit != 268435456 {
		t.Errorf("memory: %+v", u)
	}
	// 0.4 s of CPU over 10 s is 0.04 cores, of a 0.5 core limit.
	if !u.CPUKnown || u.CPUCores < 0.0399 || u.CPUCores > 0.0401 ||
		u.CPULimit != 0.5 {
		t.Errorf("cpu: %+v", u)
	}
	if u.DiskKnown {
		t.Errorf("disk without a directory: %+v", u)
	}
}

func TestUsageReaderCgroupV2WithoutLimits(t *testing.T) {
	root := writeCgroup(t, map[string]string{
		"memory.current": "1024", "memory.max": "max",
		"cpu.max": "max 100000", "cpu.stat": "usage_usec 5",
	})
	r := NewUsageReader(root, "", steps(time.Second))
	r.Sample()
	r.Sample()
	u := r.Use()
	if !u.MemoryKnown || u.MemoryLimit != 0 || !u.CPUKnown ||
		u.CPULimit != 0 {
		t.Errorf("no limits: %+v", u)
	}
}

func TestUsageReaderCgroupV1(t *testing.T) {
	root := writeCgroup(t, map[string]string{
		"memory/memory.usage_in_bytes": "2048",
		"memory/memory.limit_in_bytes": "9223372036854771712",
		"cpu/cpu.cfs_quota_us":         "200000",
		"cpu/cpu.cfs_period_us":        "100000",
		"cpuacct/cpuacct.usage":        "1000000000",
	})
	r := NewUsageReader(root, "", steps(time.Second))
	r.Sample()
	if err := os.WriteFile(filepath.Join(root, "cpuacct/cpuacct.usage"),
		[]byte("1500000000"), 0o600); err != nil {
		t.Fatal(err)
	}
	r.Sample()
	u := r.Use()
	if !u.MemoryKnown || u.MemoryBytes != 2048 || u.MemoryLimit != 0 {
		t.Errorf("memory: %+v", u)
	}
	if !u.CPUKnown || u.CPUCores != 0.5 || u.CPULimit != 2 {
		t.Errorf("cpu: %+v", u)
	}
}

func TestUsageReaderMissingFilesAreUnknown(t *testing.T) {
	r := NewUsageReader(t.TempDir(), "", steps(time.Second))
	r.Sample()
	r.Sample()
	if u := r.Use(); u.MemoryKnown || u.CPUKnown || u.DiskKnown {
		t.Errorf("nothing to read: %+v", u)
	}
}

func TestUsageReaderDiskAndRun(t *testing.T) {
	r := NewUsageReader(t.TempDir(), t.TempDir(), steps(time.Second))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		r.Run(ctx, make(chan time.Time))
		close(done)
	}()
	cancel()
	<-done
	u := r.Use()
	if !u.DiskKnown || u.DiskTotal <= 0 || u.DiskUsed > u.DiskTotal {
		t.Errorf("disk: %+v", u)
	}
}
