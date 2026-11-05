package incident

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
)

// A repeated Warning event on a root that already fails on its own only
// confirms the failure: never news, whatever its message. On a root
// with nothing else it is the finding itself and counts.
func TestManagerFingerprintSkipsARepeatedEventOnAFailingRoot(t *testing.T) {
	for name, message := range map[string]string{
		"same message":    "Secret does not exist",
		"another message": "quota exceeded",
	} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t, Config{})
			root := inventory.EntityID{Group: "example.com",
				Kind: "issuer", Namespace: "shop", Name: "internal"}
			failing := sig(root, reasons.CustomResourceFailure,
				detection.Warning)
			failing.Evidence = []detection.Evidence{
				{Label: "event", Value: "Secret does not exist"}}
			announced(t, r, failing)

			repeat := sig(root, reasons.UnusualEvent("UpdateFailed"),
				detection.Warning)
			repeat.Evidence = []detection.Evidence{
				{Label: "event", Value: message},
				{Label: "occurrences", Value: "3"}}
			r.raise(at(2*time.Minute), repeat)
			ds := r.tick(at(3 * time.Minute))
			wantNone(t, ds)
		})
	}
}

// A workload already told as not ready that later shows pods that run
// but never get ready is no news; a first rollout that never became
// healthy is.
func TestManagerFingerprintNeverReadyAfterNotReady(t *testing.T) {
	for name, firstRollout := range map[string]bool{
		"stopped being ready": false,
		"first rollout":       true,
	} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t, Config{})
			root := inventory.NewEntityID("apps", "deployment", "shop", "web")
			announced(t, r, sig(root, reasons.ContainersNotReady,
				detection.Warning))
			never := sig(root, reasons.WorkloadNeverReady, detection.Warning)
			if firstRollout {
				never.Evidence = []detection.Evidence{{
					Label: detection.EvidenceNeverHealthy, Value: "x"}}
			}
			r.raise(at(2*time.Minute), never)
			ds := r.tick(at(3 * time.Minute))
			if !firstRollout {
				wantNone(t, ds)
				return
			}
			wantAction(t, ds, Update, "material change")
		})
	}
}

// A repeat left out as restated stays left out when what it confirmed
// clears: the incident getting better is no update. Both the repeated
// event and the never-ready finding, as in an independent review.
func TestManagerRestatedReasonStaysQuietWhenItsCauseClears(t *testing.T) {
	for name, kinds := range map[string][2]string{
		"repeated event": {reasons.CustomResourceFailure,
			reasons.UnusualEvent("UpdateFailed")},
		"never ready": {reasons.ContainersNotReady,
			reasons.WorkloadNeverReady},
	} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t, Config{})
			root := inventory.NewEntityID("apps", "deployment", "shop", "web")
			main := sig(root, kinds[0], detection.Warning)
			announced(t, r, main)
			repeat := sig(root, kinds[1], detection.Warning)
			r.raise(at(2*time.Minute), repeat)
			wantNone(t, r.tick(at(3*time.Minute)))
			r.clear(at(4*time.Minute), main)
			wantNone(t, r.tick(at(5*time.Minute)))
		})
	}
}
