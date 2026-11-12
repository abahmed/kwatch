package explain

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// startLine is the first error line every workload logs at start-up.
const startLine = "CacheConnectionException: it was not possible to " +
	"connect to the cache servers"

// failWith gives id a failing finding of mode with no error of its own.
func (f *fixture) failWith(id inventory.EntityID, mode detection.Mode) {
	f.findings[id] = append(f.findings[id], detection.Finding{
		Entity: id, Reason: string(mode), Mode: mode, Health: failingH,
		Since: t0.Add(2 * time.Minute), Summary: string(mode)})
}

// mixedStartStorm fails one workload per mode, each container's last
// run having logged startLine: a crash with the line as its error, a
// liveness kill and a pod that never turns ready, with the line only
// in the previous log.
func mixedStartStorm(f *fixture) []inventory.EntityID {
	crash := containerOf(f.workload("shop", "cart", 1)[0])
	f.failError(crash, detection.ModeCrashLoop, startLine)
	live := containerOf(f.workload("shop", "orders", 1)[0])
	f.failWith(live, detection.ModeProbeLiveness)
	slow := f.workload("media", "thumbs", 1)[0]
	f.failWith(slow, detection.ModeNotReady)
	for _, c := range []inventory.EntityID{live, containerOf(slow)} {
		f.observe(c, map[string]inventory.Value{
			kube.AttrLastErrorLine: inventory.Text(startLine)})
	}
	return []inventory.EntityID{crash, live, slow}
}

// One error at start-up shows as a crash, a liveness kill and a pod
// never ready: the modes differ, the error line is one cause.
func TestStartErrorSharedAcrossModesIsOneCause(t *testing.T) {
	f := newFixture(t)
	effects := mixedStartStorm(f)

	e := f.explain()

	want := "failure-signature//cacheconnectionexception: it was not " +
		"possible to connect to the cache servers"
	for _, effect := range effects {
		requireCause(t, e, effect, want)
	}
}

// A liveness kill with no error line of its own shares nothing.
func TestProbeFailureWithoutErrorLineSharesNothing(t *testing.T) {
	f := newFixture(t)
	for _, name := range []string{"cart", "orders", "search"} {
		f.failWith(containerOf(f.workload("shop", name, 1)[0]),
			detection.ModeProbeLiveness)
	}

	for _, area := range f.explain().Areas {
		for _, c := range area.Causes {
			if c.Root.Kind == KindFailureSignature {
				t.Fatalf("signature without an error line: %v", c.Root)
			}
		}
	}
}
