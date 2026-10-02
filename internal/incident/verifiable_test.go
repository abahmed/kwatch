package incident

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// kindSight is a switchable Verifiable function.
type kindSight struct{ hidden map[inventory.Kind]bool }

func (s *kindSight) verifiable(kind inventory.Kind) bool {
	return !s.hidden[kind]
}

func TestManagerRestoredIncidentIsNotResolvedWhileUnverifiable(
	t *testing.T,
) {
	old := newRig(t, Config{})
	announced(t, old, podSig("web"))

	sight := &kindSight{hidden: map[inventory.Kind]bool{kube.KindPod: true}}
	fresh := newRig(t, Config{Verifiable: sight.verifiable})
	fresh.m.Restore(old.m.Export(), at(10*time.Minute))

	wantNone(t, fresh.tick(at(10*time.Minute)))
	wantNone(t, fresh.tick(at(30*time.Minute)))
	if fresh.only().State != Open {
		t.Fatal("an incident whose kind cannot be observed must stay open")
	}

	sight.hidden = nil
	wantNone(t, fresh.tick(at(31*time.Minute)))
	if fresh.only().State != Recovering {
		t.Fatal("once observable again, the incident may recover")
	}
	ds := fresh.tick(at(31*time.Minute + DefaultHold))
	wantAction(t, ds, Resolve, "healthy for 3m0s")
}

func TestManagerRecoveringIncidentHoldsWhenRootGoesOutOfSight(
	t *testing.T,
) {
	sight := &kindSight{hidden: map[inventory.Kind]bool{}}
	r := newRig(t, Config{Verifiable: sight.verifiable})
	web := podSig("web")
	announced(t, r, web)
	r.clear(at(5*time.Minute), web)
	wantNone(t, r.tick(at(5*time.Minute)))
	if r.only().State != Recovering {
		t.Fatal("cleared incident should be recovering")
	}

	sight.hidden[kube.KindPod] = true
	wantNone(t, r.tick(at(5*time.Minute+DefaultHold)))
	if r.only().State != Recovering {
		t.Fatal("recovery must not complete on missing data")
	}

	sight.hidden = nil
	wantNone(t, r.tick(at(9*time.Minute)))
	ds := r.tick(at(5*time.Minute + 2*DefaultHold))
	wantAction(t, ds, Resolve, "healthy for 3m0s")
}

func TestManagerSettlingIncidentWaitsWhileUnverifiable(t *testing.T) {
	old := newRig(t, Config{})
	old.raise(at(0), podSig("web"))

	sight := &kindSight{hidden: map[inventory.Kind]bool{kube.KindPod: true}}
	fresh := newRig(t, Config{Verifiable: sight.verifiable})
	fresh.m.Restore(old.m.Export(), at(10*time.Minute))
	wantNone(t, fresh.tick(at(20*time.Minute)))
	if fresh.only().State != Settling {
		t.Fatal("settling incident must not resolve on missing data")
	}
}
