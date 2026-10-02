package incident

import (
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// announceWith raises a crash on a fresh manager with nonce and returns
// the announcement decision.
func announceWith(t *testing.T, nonce string) Decision {
	t.Helper()
	r := newRig(t, Config{IDNonce: nonce})
	r.raise(at(0), podSig("web"))
	for _, d := range r.tick(at(10 * time.Minute)) {
		if d.Action == Announce {
			return d
		}
	}
	t.Fatal("the incident was never announced")
	return Decision{}
}

// After a store reset the incident ID changes (new nonce) but the alert
// key does not, so the paging alert left open before the reset is
// updated and resolved instead of a second one being opened.
func TestAlertKeySurvivesStoreReset(t *testing.T) {
	before := announceWith(t, "aaaa").Incident
	after := announceWith(t, "bbbb").Incident

	if before.ID == after.ID {
		t.Fatalf("IDs must differ across a reset: %s", before.ID)
	}
	if before.AlertKey == "" || before.AlertKey != after.AlertKey {
		t.Fatalf("alert keys %q and %q, want one stable key",
			before.AlertKey, after.AlertKey)
	}
	if strings.Contains(before.AlertKey, "aaaa") {
		t.Fatalf("alert key %q must not carry the nonce", before.AlertKey)
	}
}

func TestAlertKeyIsPersistedAndDerivedForOldRecords(t *testing.T) {
	r := newRig(t, Config{})
	r.raise(at(0), podSig("web"))
	r.tick(at(10 * time.Minute))
	records := r.m.Export()
	want := records[0].AlertKey

	legacy := records[0]
	legacy.AlertKey = ""
	fresh := newRig(t, Config{})
	fresh.m.Restore([]Record{legacy}, time.Time{})

	if want == "" || fresh.m.Incidents()[0].AlertKey != want {
		t.Fatalf("restored key = %q, want %q",
			fresh.m.Incidents()[0].AlertKey, want)
	}
}

// A revision moves the incident to a new root; its alert must stay the
// same one, or the old alert would never be resolved.
func TestAlertKeyStaysWhenCauseIsRevised(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	announced(t, r, web)
	key := r.of(web.Entity).AlertKey
	node := entity(kube.KindNode, "n1")

	r.cause(web.Entity, node, "node n1 is out of memory")
	r.apply(at(2*time.Minute), detection.Changed, web)
	ds := r.tick(at(2*time.Minute + DefaultReviseSettle))

	wantAction(t, ds, Update, ReasonCauseRevised)
	if got := ds[0].Incident; got.Root != node || got.AlertKey != key {
		t.Fatalf("revised alert key = %q on %v, want %q", got.AlertKey,
			got.Root, key)
	}
}
