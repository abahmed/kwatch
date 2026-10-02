package incident

import (
	"fmt"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A storm of short blips over many roots keeps at most one incident per
// root: each recurrence takes over its predecessor's history.
func TestManagerBlipStormStaysBounded(t *testing.T) {
	r := newRig(t, Config{})
	const roots, blips = 100, 5
	now := time.Duration(0)
	for blip := 0; blip < blips; blip++ {
		var sigs []detection.Finding
		for i := 0; i < roots; i++ {
			sigs = append(sigs, podSig(fmt.Sprintf("web-%d", i)))
		}
		r.raise(at(now), sigs...)
		r.tick(at(now))
		now += time.Second
		r.clear(at(now), sigs...)
		r.tick(at(now))
		now += time.Minute
	}
	if got := len(r.m.Export()); got > roots {
		t.Fatalf("kept %d incidents for %d roots", got, roots)
	}
	last := r.of(entity(kube.KindPod, "web-0"))
	if len(last.Occurrences) != blips || last.Previous == "" {
		t.Fatalf("recurrence lost its history: %+v", last)
	}
}

func resolvedIncident(m *Manager, id, root string, at time.Time) {
	p := &Incident{ID: id, Root: entity(kube.KindDeployment, root),
		State: Resolved, Resolved: at}
	m.incidents[id] = p
	m.index(p)
	m.markResolved(p)
}

func TestManagerBoundsResolvedIncidentsPerRootAndInTotal(t *testing.T) {
	m := NewManager(Config{}, &stubExplainer{})
	for i := 0; i < 5; i++ {
		resolvedIncident(m, fmt.Sprintf("same-%d", i), "web", at(0))
	}
	if got := len(m.incidents); got != maxResolvedPerRoot {
		t.Fatalf("kept %d for one root, want %d", got, maxResolvedPerRoot)
	}
	if m.incidents["same-0"] != nil || m.incidents["same-4"] == nil {
		t.Fatal("the oldest of a root must go first")
	}

	m = NewManager(Config{}, &stubExplainer{})
	for i := 0; i < maxResolved+5; i++ {
		resolvedIncident(m, fmt.Sprintf("inc-%05d", i),
			fmt.Sprintf("web-%d", i), at(time.Duration(i)*time.Second))
	}
	if got := len(m.incidents); got != maxResolved {
		t.Fatalf("kept %d, want %d", got, maxResolved)
	}
	if m.incidents["inc-00004"] != nil || m.incidents["inc-00005"] == nil {
		t.Fatal("the oldest resolved incidents must go first")
	}
}

func TestManagerTickExpiresResolvedInOrder(t *testing.T) {
	m := NewManager(Config{Remember: time.Hour}, &stubExplainer{})
	resolvedIncident(m, "a", "web-a", at(0))
	resolvedIncident(m, "b", "web-b", at(30*time.Minute))
	_, next := m.Tick(at(time.Hour))
	if next != time.Nanosecond {
		t.Fatalf("next wake = %v, want the first expiry", next)
	}
	m.Tick(at(time.Hour + time.Second))
	if m.incidents["a"] != nil || m.incidents["b"] == nil {
		t.Fatal("only the incident past remember may expire")
	}
}

// A root that flaps for a long time leaves one stale ID in the resolve
// order per cycle. An old resolved incident at the front keeps those IDs
// from being dropped there, so compaction must keep the order bounded.
func TestManagerLongFlappingRunKeepsResolvedOrderBounded(t *testing.T) {
	r := newRig(t, Config{})
	resolvedIncident(r.m, "old", "other", at(0))
	web := podSig("web")
	now := time.Second
	for cycle := 0; cycle < 2000; cycle++ {
		r.raise(at(now), web)
		r.tick(at(now))
		now += time.Second
		r.clear(at(now), web)
		r.tick(at(now))
		now += time.Second
	}
	count := r.m.resolvedCount()
	if count != 2 {
		t.Fatalf("resolved count = %d, want the old one and web", count)
	}
	if got := len(r.m.resolved.order); got > minCompactOrder {
		t.Fatalf("order holds %d IDs for %d resolved", got, count)
	}
	if p := r.m.oldestResolved(); p == nil || p.ID != "old" {
		t.Fatalf("oldest resolved = %+v, want old", p)
	}
}

func TestResolvedIndexCompactKeepsLastOccurrenceInOrder(t *testing.T) {
	idx := newResolvedIndex()
	idx.byRoot["a"] = []string{"x"}
	idx.byRoot["b"] = []string{"y"}
	idx.count = 2
	for i := 0; i < minCompactOrder; i++ {
		idx.order = append(idx.order, fmt.Sprintf("stale-%d", i))
	}
	idx.order = append(idx.order, "y", "x", "y")
	idx.compact()
	if len(idx.order) != 2 || idx.order[0] != "x" || idx.order[1] != "y" {
		t.Fatalf("order = %v, want [x y]", idx.order)
	}
}
