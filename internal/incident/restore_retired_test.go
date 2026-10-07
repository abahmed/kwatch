package incident

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
)

// retiredNames are reasons and modes older versions saved that no
// detector raises any more.
var retiredNames = []struct{ reason, mode string }{
	{"NodeResourceHigh", "ResourceHigh"},
	{"NodeResourceCritical", "ResourceCritical"},
	{"VolumeFillingUp", "VolumeFillingUp"},
	{"Risk.SingleNode", "Risk.SingleNode"},
	{"Risk.Privileged", "Risk.Privileged"},
	{"Risk.ConfigVersionSkew", "Risk.ConfigVersionSkew"},
}

// A record saved by an older version, with a reason or mode that no
// longer exists, restores and resolves like any other: it is never
// stuck open waiting for a finding that cannot come back.
func TestRestoredRetiredReasonsStillResolve(t *testing.T) {
	for _, old := range retiredNames {
		t.Run(old.reason, func(t *testing.T) {
			raw := `{"Mode":"` + old.mode + `","Modes":["` + old.mode +
				`"],"RootReasons":["` + old.reason + `"]}`
			got := restoreAfterGap(t, func(r *Record) {
				if err := json.Unmarshal([]byte(raw), r); err != nil {
					t.Fatal(err)
				}
			}, 8*time.Hour, 10*time.Minute)
			if got.IsZero() {
				t.Fatal("never resolved")
			}
		})
	}
}

// A finding that carries a reason no detector knows (a custom one, or
// one from an older version) opens and resolves like any other; the
// policy has no entry for it and treats it as a plain warning.
func TestUnknownReasonIncidentOpensAndResolves(t *testing.T) {
	for _, old := range retiredNames {
		t.Run(old.reason, func(t *testing.T) {
			r := newRig(t, Config{})
			f := sig(entity("Node", "n1"), old.reason, detection.Warning)
			r.raise(at(0), f)
			wantAction(t, r.tick(at(DefaultSettle)), Announce, "settled")
			r.clear(at(time.Minute), f)
			for i := range 20 {
				ds := r.tick(at(2*time.Minute + time.Duration(i)*time.Minute))
				for _, d := range ds {
					if d.Action == Resolve {
						return
					}
				}
			}
			t.Fatal("incident never resolved")
		})
	}
}
