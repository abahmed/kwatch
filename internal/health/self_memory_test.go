package health

import (
	"testing"
	"time"
)

// The level changes only after the new one held for memoryHold, and
// is left only below the exit line, so a value near a line is steady.
func TestMemoryAlarmHoldsAndHasHysteresis(t *testing.T) {
	t0 := time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)
	var a memoryAlarm
	at := func(d time.Duration, share float64) string {
		return a.observe(t0.Add(d), int64(share*1000), 1000)
	}
	steps := []struct {
		at    time.Duration
		share float64
		want  string
	}{
		{0, 0.76, MemoryOK},
		{time.Minute, 0.82, MemoryOK},
		{2 * time.Minute, 0.70, MemoryOK},    // dipped: hold restarts
		{3 * time.Minute, 0.81, MemoryOK},    // above again from here
		{5 * time.Minute, 0.83, MemoryOK},    // two minutes only
		{6 * time.Minute, 0.81, MemoryWarn},  // held three minutes
		{7 * time.Minute, 0.72, MemoryWarn},  // above the exit line
		{8 * time.Minute, 0.69, MemoryWarn},  // below: pending
		{9 * time.Minute, 0.76, MemoryWarn},  // back above the exit
		{10 * time.Minute, 0.60, MemoryWarn}, // pending again
		{13 * time.Minute, 0.60, MemoryOK},   // held: cleared
		{14 * time.Minute, 0.97, MemoryOK},   // pending bad
		{17 * time.Minute, 0.96, MemoryBad},  // held
		{18 * time.Minute, 0.91, MemoryBad},  // above the bad exit
		{19 * time.Minute, 0.85, MemoryBad},  // pending warn
		{22 * time.Minute, 0.85, MemoryWarn}, // held
		{23 * time.Minute, 0.50, MemoryWarn}, // pending ok
		{26 * time.Minute, 0.50, MemoryOK},   // held
		{27 * time.Minute, 0.50, MemoryOK},   // steady
		{28 * time.Minute, 2.00, MemoryOK},   // a spike starts pending
		{29 * time.Minute, 0.10, MemoryOK},   // and is forgotten
		{40 * time.Minute, 0.10, MemoryOK},   // no stale pending
		{41 * time.Minute, 0.90, MemoryOK},   // warn pending
		{44 * time.Minute, 0.90, MemoryWarn}, // held
		{45 * time.Minute, 0.00, MemoryWarn}, // pending ok
		{46 * time.Minute, 0.95, MemoryWarn}, // now pending bad
		{47 * time.Minute, 0.95, MemoryWarn}, // only one minute
		{49 * time.Minute, 0.95, MemoryBad},  // held from 46
		{50 * time.Minute, 0.95, MemoryBad},  // steady
		{51 * time.Minute, 0.00, MemoryBad},  // pending ok
		{54 * time.Minute, 0.00, MemoryOK},   // held
	}
	for _, s := range steps {
		if got := at(s.at, s.share); got != s.want {
			t.Fatalf("at %v share %.2f: level %q, want %q",
				s.at, s.share, got, s.want)
		}
	}
}

func TestMemoryAlarmNeedsABase(t *testing.T) {
	var a memoryAlarm
	t0 := time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)
	for i := range 10 {
		if got := a.observe(t0.Add(time.Duration(i)*time.Minute),
			1<<40, 0); got != MemoryOK {
			t.Fatalf("no limit judged %q", got)
		}
	}
}

func TestMemoryBasePrefersTheGoLimit(t *testing.T) {
	cases := []struct{ goLimit, limit, want int64 }{
		{900, 1000, 900},
		{0, 1000, 1000},
		{2000, 1000, 1000},
		{900, 0, 900},
		{0, 0, 0},
	}
	for _, c := range cases {
		if got := memoryBase(c.goLimit, c.limit); got != c.want {
			t.Errorf("memoryBase(%d, %d) = %d, want %d",
				c.goLimit, c.limit, got, c.want)
		}
	}
}

// The working set counts the state file's cache; the level reads the
// Go memory, so a full cache alone raises nothing.
func TestUsageReaderJudgesGoMemoryNotTheCache(t *testing.T) {
	root := writeCgroup(t, map[string]string{
		"memory.current": "800",
		"memory.max":     "1000",
		"memory.stat":    "anon 300\nfile 450\nkernel 50",
	})
	r := NewUsageReader(root, "", steps(time.Minute))
	goUsed := int64(400)
	r.goMem = func() (int64, int64) { return goUsed, 900 }
	for range 6 {
		r.Sample()
	}
	u := r.Use()
	if u.MemoryBytes != 800 || u.MemoryCache != 450 ||
		!u.GoMemoryKnown || u.GoMemory != 400 || u.GoLimit != 900 {
		t.Fatalf("memory: %+v", u)
	}
	if u.MemoryLevel != MemoryOK {
		t.Fatalf("80%% working set, 44%% Go memory: level %q",
			u.MemoryLevel)
	}
	goUsed = 760 // 84% of the Go limit
	for range 5 {
		r.Sample()
	}
	if u = r.Use(); u.MemoryLevel != MemoryWarn {
		t.Fatalf("84%% of the Go limit for 4 minutes: %q", u.MemoryLevel)
	}
}

func TestUsageReaderReadsCgroupV1Cache(t *testing.T) {
	root := writeCgroup(t, map[string]string{
		"memory/memory.usage_in_bytes": "500",
		"memory/memory.limit_in_bytes": "1000",
		"memory/memory.stat":           "cache 120\nrss 300\ntotal_cache 200",
	})
	r := NewUsageReader(root, "", steps(time.Second))
	if u := r.Use(); u.MemoryCache != 200 {
		t.Fatalf("v1 cache = %d, want total_cache 200", u.MemoryCache)
	}
}

func TestGoMemoryReadsTheRuntime(t *testing.T) {
	used, _ := goMemory()
	if used <= 0 {
		t.Fatalf("Go memory = %d", used)
	}
}
