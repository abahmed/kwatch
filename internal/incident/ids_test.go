package incident

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestManagerIDsAreUniqueAndDeterministic(t *testing.T) {
	run := func() []string {
		r := newRig(t, Config{})
		r.raise(at(0), podSig("web"), podSig("api"))
		r.raise(at(24*time.Hour), podSig("db"))
		var ids []string
		for _, rec := range r.m.Export() {
			ids = append(ids, rec.ID)
		}
		return ids
	}

	first, second := run(), run()
	want := []string{
		"inc-20260105-7f3a-0001", "inc-20260105-7f3a-0002",
		"inc-20260106-7f3a-0003",
	}
	if len(first) != len(want) {
		t.Fatalf("ids = %v, want %v", first, want)
	}
	for i := range want {
		if first[i] != want[i] || second[i] != want[i] {
			t.Fatalf("ids = %v and %v, want %v", first, second, want)
		}
	}
}

func TestManagerRestoreContinuesIDSequence(t *testing.T) {
	old := newRig(t, Config{})
	old.raise(at(0), podSig("web"), podSig("api"))

	fresh := newRig(t, Config{})
	fresh.m.Restore(old.m.Export(), time.Time{})
	fresh.raise(at(time.Minute), podSig("db"))

	got := fresh.idOf(podSig("db").Entity)
	if got != "inc-20260105-7f3a-0003" {
		t.Fatalf("new id = %s, want the sequence to continue", got)
	}
}

// A store reset, or a new empty volume after a reschedule, starts the
// sequence over on the same day. The new store's nonce keeps every new
// ID different from the IDs the old store handed out.
func TestManagerIDsAfterStoreResetNeverRepeat(t *testing.T) {
	old := newRig(t, Config{IDNonce: "aaaa"})
	old.raise(at(0), podSig("web"), podSig("api"))
	before := map[string]bool{}
	for _, rec := range old.m.Export() {
		before[rec.ID] = true
	}

	reset := newRig(t, Config{IDNonce: "bbbb"})
	reset.raise(at(time.Minute), podSig("web"), podSig("api"))

	for _, rec := range reset.m.Export() {
		if before[rec.ID] {
			t.Fatalf("id %s repeats after a reset", rec.ID)
		}
	}
}

func TestManagerRestoreAdoptsStoredNonce(t *testing.T) {
	old := newRig(t, Config{IDNonce: "aaaa"})
	old.raise(at(0), podSig("web"))

	fresh := newRig(t, Config{IDNonce: "bbbb"})
	fresh.m.Restore(old.m.Export(), time.Time{})
	fresh.raise(at(time.Minute), podSig("db"))

	got := fresh.idOf(podSig("db").Entity)
	if got != "inc-20260105-aaaa-0002" {
		t.Fatalf("new id = %s, want the stored nonce and sequence", got)
	}
}

func TestManagerRestoreOfIDsWithoutNonceKeepsOwnNonce(t *testing.T) {
	fresh := newRig(t, Config{IDNonce: "bbbb"})
	fresh.m.Restore([]Record{{
		ID: "inc-20260105-0004", Root: podSig("web").Entity,
		State: Resolved, Resolved: at(0),
	}}, time.Time{})
	fresh.raise(at(time.Minute), podSig("db"))

	got := fresh.idOf(podSig("db").Entity)
	if got != "inc-20260105-bbbb-0005" {
		t.Fatalf("new id = %s, want own nonce after the old sequence", got)
	}
}

func TestRandomNonceIsShortHexAndVaries(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 64; i++ {
		n := randomNonce()
		if idNonce("inc-20260105-"+n+"-0001") != n {
			t.Fatalf("nonce %q is not 4 hex characters", n)
		}
		seen[n] = true
	}
	if len(seen) < 2 {
		t.Fatal("64 random nonces were all equal")
	}
	if m := NewManager(Config{}, nil); idNonce(
		"inc-20260105-"+m.nonce+"-0001") == "" {
		t.Fatalf("manager nonce %q is not valid", m.nonce)
	}
}

func TestManagerRestoreRoundTripKeepsIDRootAndLink(t *testing.T) {
	src := newRig(t, Config{})
	web := podSig("web")
	announced(t, src, web)
	src.clear(at(2*time.Minute), web)
	src.tick(at(2 * time.Minute))
	src.tick(at(2*time.Minute + DefaultHold))
	src.raise(at(time.Hour), web)
	want := src.of(web.Entity)

	dst := newRig(t, Config{})
	dst.m.Restore(src.m.Export(), time.Time{})
	got := dst.of(web.Entity)

	if got.ID != want.ID || got.Root != want.Root ||
		got.Previous != want.Previous || got.Previous == "" {
		t.Fatalf("restored %+v, want %+v", got, want)
	}
}

func TestManagerRestoreIndexesLiveIncidentOverResolved(t *testing.T) {
	src := newRig(t, Config{})
	web := podSig("web")
	announced(t, src, web)
	node := entity(kube.KindNode, "n1")
	src.cause(web.Entity, node, "node down")
	src.apply(at(2*time.Minute), detection.Changed, web)
	live := src.idOf(node)
	records := src.m.Export()
	stale := records[0]
	stale.ID, stale.State = "inc-20260105-0009", Resolved
	stale.Opened = at(time.Hour)

	dst := newRig(t, Config{})
	dst.m.Restore(append(records, stale), time.Time{})

	if got := dst.idOf(node); got != live {
		t.Fatalf("root indexed to %s, want live %s", got, live)
	}
}
