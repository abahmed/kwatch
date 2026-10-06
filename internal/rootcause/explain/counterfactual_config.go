package explain

import (
	"fmt"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// scoreConfigElsewhere asks whether a ConfigMap or Secret blamed for a
// failure is also used by a healthy workload. The same object works
// there, so it is less likely to be what broke this one. Healthy pods
// of the failing workload itself are not counted: coverage already
// weighs them.
func scoreConfigElsewhere(v *view, c *candidate) outcome {
	if c.id.Kind != kube.KindConfigMap && c.id.Kind != kube.KindSecret {
		return outcome{}
	}
	units := v.failingUnits(c)
	failing := v.workloadsOfUnits(units)
	for _, user := range sampleIDs(v.dependents(c.id)) {
		pod, ok := v.podOf(user)
		if !ok || failing[v.workloadOf(pod)] || !v.healthyWorkload(pod) {
			continue
		}
		twin := v.workloadOf(pod)
		text := fmt.Sprintf("%s %s is used by healthy %s too, so "+
			"it works elsewhere", c.id.Kind, shortName(c.id),
			shortName(twin))
		for _, u := range units {
			v.compare(u.effect, Comparison{
				Code: rootcause.ProofConfigElsewhere, With: twin,
				Text: text})
		}
		return outcome{weight: -ConfigElsewhereWeight,
			code: rootcause.ProofConfigElsewhere, text: text}
	}
	return outcome{}
}

// scorePreviousHealthy rewards a rollout whose previous revision ran
// healthy, for at least PreviousHealthyMin, before the change: what
// differs now is the change. It needs a pod of the previous revision
// that is still up; a revision already scaled away leaves no evidence.
func scorePreviousHealthy(v *view, c *candidate) outcome {
	if c.bestMatch().match.row.Name != "rollout" {
		return outcome{}
	}
	changedAt := v.latestChange(c.id)
	if changedAt.IsZero() {
		return outcome{}
	}
	previous := v.previousPods(c.id)
	if len(previous) == 0 || v.anyFailing(previous) {
		return outcome{}
	}
	minutes := 0
	for _, pod := range previous {
		ran := v.readyBefore(pod, changedAt)
		if ran >= PreviousHealthyMin {
			minutes = int(ran.Minutes())
			break
		}
	}
	if minutes == 0 {
		return outcome{}
	}
	text := fmt.Sprintf("the previous revision ran healthy for %d "+
		"minutes before the change", minutes)
	for _, effect := range c.direct() {
		v.compare(effect, Comparison{Code: rootcause.ProofPreviousHealthy,
			With: c.id, Count: minutes, Text: text})
	}
	return outcome{weight: PreviousHealthyWeight, count: minutes,
		code: rootcause.ProofPreviousHealthy, text: text}
}

// previousPods lists the pods of the revisions older than the newest
// one of a Deployment.
func (v *view) previousPods(
	deployment inventory.EntityID,
) []inventory.EntityID {
	newest, ok := 0, false
	revisions := map[inventory.EntityID]int{}
	for _, rs := range v.s.Model.Related(
		deployment, inventory.OwnedBy, inventory.Incoming,
	) {
		if n, found := revisionOf(v.s.Model, rs); found {
			revisions[rs] = n
			if !ok || n > newest {
				newest, ok = n, true
			}
		}
	}
	var out []inventory.EntityID
	for rs, n := range revisions {
		if n < newest {
			out = append(out, v.ownedPods(rs)...)
		}
	}
	sortIDs(out)
	return sampleIDs(out)
}

// latestChange is when id last changed inside the causal window; zero
// when it did not.
func (v *view) latestChange(id inventory.EntityID) (at time.Time) {
	for _, change := range v.changesOf(id) {
		if change.At.After(at) {
			at = change.At
		}
	}
	return at
}

// readyBefore is how long before t the pod had been ready; zero when
// it was not ready then or its ready time is unknown.
func (v *view) readyBefore(pod inventory.EntityID, t time.Time) time.Duration {
	e, ok := v.s.Model.Entity(pod)
	if !ok {
		return 0
	}
	since, ok := attrTime(e, kube.AttrReadySince)
	if !ok || since.After(t) {
		return 0
	}
	return t.Sub(since)
}
