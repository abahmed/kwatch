package release

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const (
	// WatchWindow is how long a rolled-out release is watched before it
	// is called healthy, as long as kwatch's release watch compares a
	// new revision with the previous one.
	WatchWindow = 15 * time.Minute
	// StallAfter is how long a rollout of a kind without a progress
	// deadline (StatefulSets and DaemonSets) may go without any replica
	// being updated or getting ready before it counts as stalled. A
	// Deployment stalls only by its own progress deadline.
	StallAfter = 15 * time.Minute
)

// judge updates r's rollout state and verdict. next is the start of the
// workload's next release, or zero.
func judge(r *Record, v View, next time.Time, now time.Time) {
	id := inventory.CoreID(inventory.Kind(r.Kind), r.Namespace, r.Name)
	e, ok := v.Model.Entity(id)
	if !ok {
		// Deleted: what it reached stands.
		r.Final = true
		return
	}
	st := rolloutOf(e, inventory.Kind(r.Kind))
	r.Ready, r.Desired = st.ready, st.desired
	r.OldServing = st.ready > 0 && st.updated < st.desired
	if st.complete && r.RolledOut.IsZero() {
		r.RolledOut = now
	}
	r.Deadline = st.deadline
	r.Stalled = st.deadline || stalledWithoutDeadline(r, st, now)
	verdict, incidentID := incidentVerdict(r, v, id, next)
	if verdict == "" && r.Stalled {
		verdict = Failed
	}
	if verdict == Failed && st.desired > 0 && st.ready == 0 {
		verdict = Down
	}
	if incidentID != "" {
		r.Incident = incidentID
	}
	if v.Detail {
		r.Detail = detailOf(v.Model, id, *r, now)
	}
	r.Verdict, r.Final = settle(*r, verdict, st.complete, next, now)
}

// settle is the verdict after a look that found bad (or nothing), and
// whether it is final. A bad verdict found inside the window stays the
// worst one reached; a newer release ends the watch.
func settle(
	r Record, bad Verdict, complete bool, next time.Time, now time.Time,
) (Verdict, bool) {
	horizon := r.At.Add(StallAfter + WatchWindow)
	if bad.severity() < r.Verdict.severity() {
		bad = r.Verdict
	}
	switch {
	case bad.Bad():
		return bad, !next.IsZero() || now.After(horizon)
	case !next.IsZero():
		return Replaced, true
	case !complete:
		return Rolling, false
	case now.Sub(r.RolledOut) < WatchWindow:
		return Watching, false
	}
	return Healthy, true
}

// stalledWithoutDeadline reports a StatefulSet or DaemonSet rollout
// that stopped moving: no replica updated or got ready for StallAfter.
// A rollout that waits for people on purpose (OnDelete, a StatefulSet
// partition holding ordinals back) never stalls. A Deployment has its
// own progress deadline and is not judged here.
func stalledWithoutDeadline(r *Record, st rollout, now time.Time) bool {
	if st.complete || st.byPeople ||
		r.Kind == string(kube.KindDeployment) {
		return false
	}
	p := st.updated + st.ready
	switch {
	case r.ProgressAt.IsZero():
		// First look: the rollout has been moving since the release.
		r.Progress, r.ProgressAt = p, r.At
	case p != r.Progress:
		r.Progress, r.ProgressAt = p, now
	}
	return now.Sub(r.ProgressAt) >= StallAfter
}

// deadlineReason is the Progressing reason of a Deployment whose
// rollout missed its progress deadline.
const deadlineReason = "ProgressDeadlineExceeded"

// rollout is a workload's rollout status.
type rollout struct {
	desired, ready, updated int
	complete, deadline      bool
	// byPeople: the rollout waits for people (OnDelete, a partition).
	byPeople bool
}

func rolloutOf(e inventory.Entity, kind inventory.Kind) rollout {
	desiredAttr := kube.AttrReplicas
	if kind == kube.KindDaemonSet {
		desiredAttr = kube.AttrDesiredReplicas
	}
	st := rollout{desired: count(e, desiredAttr),
		ready:   count(e, kube.AttrReadyReplicas),
		updated: count(e, kube.AttrUpdatedReplicas)}
	available := count(e, kube.AttrAvailable)
	observed, generation := count(e, kube.AttrObservedGen),
		count(e, kube.AttrGeneration)
	// The Progressing condition says the deadline passed.
	key := kube.ConditionKey("Progressing") + kube.AttrConditionReason
	if reason, ok := e.Attribute(key); ok {
		st.deadline = reason.Value.AsText() == deadlineReason
	}
	st.complete = observed >= generation && st.updated >= st.desired &&
		st.ready >= st.desired && available >= st.desired
	if s, ok := e.Attribute(kube.AttrUpdateStrategy); ok &&
		s.Value.AsText() == "OnDelete" {
		st.byPeople = true
	}
	st.byPeople = st.byPeople || count(e, kube.AttrPartition) > 0
	return st
}

func count(e inventory.Entity, name string) int {
	a, ok := e.Attribute(name)
	if !ok {
		return 0
	}
	n, _ := a.Value.AsNumber()
	return int(n)
}

// incidentVerdict is the bad verdict an incident about the workload,
// opened while the release was watched, says, with its ID: Regressed for
// the release watch's finding, Failed for any other failure people were
// told of (Down is decided by the caller from the replicas).
func incidentVerdict(
	r *Record, v View, id inventory.EntityID, next time.Time,
) (Verdict, string) {
	end := r.At.Add(StallAfter + WatchWindow)
	if !next.IsZero() && next.Before(end) {
		end = next
	}
	verdict, found := Verdict(""), ""
	for _, p := range v.Incidents {
		if p.Opened.Before(r.At.Add(-time.Minute)) || p.Opened.After(end) ||
			!about(v.Model, p, id) || !told(p) {
			continue
		}
		bad := Failed
		if regression(p) {
			bad = Regressed
		}
		if bad.severity() > verdict.severity() {
			verdict, found = bad, p.ID
		}
	}
	return verdict, found
}

// told reports an incident people were told of, or would be: a quiet one
// that settled before alerting does not fail a release.
func told(p incident.Incident) bool {
	return p.Tier >= incident.Notify || !p.Announced.IsZero()
}

func regression(p incident.Incident) bool {
	for key := range p.Members {
		if key.Reason == reasons.ReleaseRegression {
			return true
		}
	}
	return false
}

// about reports an incident whose root or members are the workload or
// its pods and containers.
func about(
	model inventory.Reader, p incident.Incident, id inventory.EntityID,
) bool {
	if ownedBy(model, p.Root, id) {
		return true
	}
	for key := range p.Members {
		if ownedBy(model, key.Entity, id) {
			return true
		}
	}
	return false
}

// ownedBy reports e is the workload or owned by it, through at most a
// container's pod and a pod's ReplicaSet.
func ownedBy(
	model inventory.Reader, e inventory.EntityID, id inventory.EntityID,
) bool {
	for range 4 {
		if e == id {
			return true
		}
		owners := model.Related(e, inventory.OwnedBy, inventory.Outgoing)
		if len(owners) == 0 {
			return false
		}
		e = owners[0]
	}
	return false
}

// sameJudgement reports two looks at a record that read the same.
func sameJudgement(a, b Record) bool {
	return a.Verdict == b.Verdict && a.Final == b.Final &&
		a.Ready == b.Ready && a.Desired == b.Desired &&
		a.Stalled == b.Stalled && a.OldServing == b.OldServing &&
		a.Incident == b.Incident && a.RolledOut.Equal(b.RolledOut) &&
		a.Progress == b.Progress && a.ProgressAt.Equal(b.ProgressAt) &&
		sameDetail(a.Detail, b.Detail)
}
