package handler

import (
	"sync"

	"github.com/abahmed/kwatch/internal/model"
)

// reconciler remembers what each watched object was last found to be wrong
// with, so recovery can be derived rather than remembered.
//
// Every detector used to carry its own "else resolve" branch -- ninety of
// them -- and each one had to name the exact reasons that detector could have
// reported. A reason added to a detector without a matching branch produced
// an incident nothing could ever close; a branch that resolved the whole
// object closed incidents another detector was still reporting. Diffing the
// findings makes both mistakes unrepresentable: what was true last pass and
// is not true now is, by definition, what recovered.
type reconciler struct {
	mu sync.Mutex
	// last maps a watched object to the reasons reported for it on the
	// previous pass. Only currently-failing objects have an entry, so this
	// is bounded by what is broken, not by cluster size.
	last map[model.ObjectRef]map[string]struct{}
}

func newReconciler() *reconciler {
	return &reconciler{last: map[model.ObjectRef]map[string]struct{}{}}
}

// diff records the current findings for a subject and returns the reasons
// that were reported before and are not reported now.
func (r *reconciler) diff(
	subject model.ObjectRef, findings []*model.Observation,
) []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	current := make(map[string]struct{}, len(findings))
	for _, obs := range findings {
		if obs != nil && obs.Reason != "" {
			current[obs.Reason] = struct{}{}
		}
	}

	var gone []string
	for reason := range r.last[subject] {
		if _, still := current[reason]; !still {
			gone = append(gone, reason)
		}
	}
	if len(current) == 0 {
		delete(r.last, subject)
	} else {
		r.last[subject] = current
	}
	return gone
}

// forget drops a subject's history and returns what it was last reported to
// be wrong with.
func (r *reconciler) forget(subject model.ObjectRef) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var gone []string
	for reason := range r.last[subject] {
		gone = append(gone, reason)
	}
	delete(r.last, subject)
	return gone
}

// reconcile reports everything currently wrong with one object and resolves
// everything that no longer is.
//
// A pass with no findings resolves the whole subject rather than only the
// reasons this process happens to remember. That is what closes incidents
// restored from disk after a restart: they were never reported by this
// process, so a diff alone would leave them open until the stale sweep closed
// them with "not observed to recover", which would be a lie -- the object is
// healthy and kwatch is looking straight at it.
func (h *handler) reconcile(
	subject model.ObjectRef, findings []*model.Observation,
) {
	gone := h.reconciler.diff(subject, findings)
	for _, obs := range findings {
		h.observe(obs)
	}
	if len(findings) == 0 {
		h.correlator.Resolve(subject, "")
		return
	}
	for _, reason := range gone {
		h.correlator.Resolve(subject, reason)
	}
}

// workloadKinds are the subject kinds that own pods. Deleting one of these
// also ends whatever its pods were being reported for.
var workloadKinds = map[string]bool{
	"deployment":  true,
	"statefulset": true,
	"daemonset":   true,
	"job":         true,
	"cronjob":     true,
	"replicaset":  true,
}

// reconcileGone resolves everything reported for an object that is no longer
// being watched -- deleted, or moved out of scope.
//
// Deleting a workload also closes its pods' incidents. Those are keyed by the
// workload but carry the "pod" kind, so the workload's own resolve never
// reached them: nothing was left that could observe those pods recovering, so
// they sat open until the stale sweep closed them, four correlation windows
// later, as "not observed to recover" -- for a Deployment somebody had
// deliberately deleted.
func (h *handler) reconcileGone(subject model.ObjectRef) {
	h.reconciler.forget(subject)
	h.correlator.Resolve(subject, "")
	if !workloadKinds[subject.Kind] {
		return
	}
	h.correlator.Resolve(model.ObjectRef{
		Kind:      "pod",
		Namespace: subject.Namespace,
		Name:      subject.Name,
	}, "")
}
