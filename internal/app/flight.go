package app

import (
	"path/filepath"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/flight"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/storage"
)

// StatePath is the state file of this installation, which the
// replay-export command reads a copy of.
func StatePath() string {
	return filepath.Join(dataDir(), "state.db")
}

// newFlightRecorder builds the flight recorder on the state file, or nil
// when it cannot start: recording is a trail for post-mortems, and never
// a reason to stop monitoring.
func newFlightRecorder(
	state *storage.Store, now func() time.Time,
) *flight.Recorder {
	if state == nil {
		return nil
	}
	salt, err := flight.LoadSalt(state)
	if err != nil {
		klog.ErrorS(err, "flight recorder disabled", "component", "flight")
		return nil
	}
	rec, err := flight.NewRecorder(flight.NewRing(state), salt, now)
	if err != nil {
		klog.ErrorS(err, "flight recorder disabled", "component", "flight")
		return nil
	}
	return rec
}

// recordedSubmit sends observations to submit through the flight
// recorder when there is one.
func recordedSubmit(rec *flight.Recorder, submit kube.Submit) kube.Submit {
	if rec == nil {
		return submit
	}
	return kube.Submit(rec.Wrap(flight.Submit(submit)))
}
