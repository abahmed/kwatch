package release

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

var t0 = time.Date(2026, 1, 5, 10, 0, 0, 0, time.UTC)

// world is a model with one workload and the incidents of a look.
type world struct {
	m         *inventory.Model
	id        inventory.EntityID
	incidents []incident.Incident
}

func newWorld(kind inventory.Kind, name string) *world {
	return &world{m: inventory.NewModel(inventory.Options{}),
		id: inventory.CoreID(kind, "shop", name)}
}

// status sets the workload's replicas: desired, ready, updated and
// available, at generation 2, with the Progressing reason when given.
func (w *world) status(desired, ready, updated int, progressing string) {
	w.statusWith(desired, ready, updated, progressing, nil)
}

// statusWith is status with extra attributes (an update strategy, say).
func (w *world) statusWith(
	desired, ready, updated int, progressing string,
	extra map[string]inventory.Value,
) {
	desiredAttr := kube.AttrReplicas
	if w.id.Kind == kube.KindDaemonSet {
		desiredAttr = kube.AttrDesiredReplicas
	}
	attrs := map[string]inventory.Value{
		desiredAttr:              inventory.Number(float64(desired)),
		kube.AttrReadyReplicas:   inventory.Number(float64(ready)),
		kube.AttrUpdatedReplicas: inventory.Number(float64(updated)),
		kube.AttrAvailable:       inventory.Number(float64(ready)),
		kube.AttrGeneration:      inventory.Number(2),
		kube.AttrObservedGen:     inventory.Number(2),
	}
	for k, v := range extra {
		attrs[k] = v
	}
	if progressing != "" {
		key := kube.ConditionKey("Progressing")
		attrs[key] = inventory.Text("False")
		attrs[key+kube.AttrConditionReason] = inventory.Text(progressing)
	}
	w.m.Apply(inventory.Observation{Kind: inventory.Observed,
		Source: "test", At: t0, Entity: w.id, Attributes: attrs})
}

// change records a change of the workload's fields at at.
func (w *world) change(
	at time.Time, app string, fields ...inventory.FieldChange,
) {
	w.m.Apply(inventory.Observation{Kind: inventory.Changed,
		Source: "test", At: at, Entity: w.id, Change: inventory.Change{
			Entity: w.id, At: at, Actor: "kubectl-client-side-apply",
			App: app, Fields: fields}})
}

func image(before, after string) inventory.FieldChange {
	return inventory.FieldChange{Path: "containers[app].image",
		Before: before, After: after}
}

func (w *world) look(b *Book, now time.Time) {
	Observe(b, View{Model: w.m, Incidents: w.incidents}, now)
}

func TestHealthyReleaseIsWatchedThenHealthy(t *testing.T) {
	w := newWorld(kube.KindDeployment, "api")
	w.status(2, 2, 2, "")
	w.change(t0, "", image("api:1", "api:2"))
	var b Book
	w.look(&b, t0.Add(time.Minute))
	require.Len(t, b.Records, 1)
	r := b.Records[0]
	assert.Equal(t, Watching, r.Verdict)
	assert.Equal(t, []Image{{Container: "app", Before: "api:1",
		After: "api:2"}}, r.Images)
	w.look(&b, t0.Add(time.Minute+WatchWindow))
	assert.Equal(t, Healthy, b.Records[0].Verdict)
	assert.True(t, b.Records[0].Final)
}

func TestScaleOnlyChangeIsNoRelease(t *testing.T) {
	w := newWorld(kube.KindDeployment, "api")
	w.status(3, 3, 3, "")
	w.change(t0, "", inventory.FieldChange{Path: "spec.replicas",
		Before: "2", After: "3"})
	var b Book
	w.look(&b, t0.Add(time.Minute))
	assert.Empty(t, b.Records)
}

func TestStalledRolloutFailsOrIsDown(t *testing.T) {
	for name, tc := range map[string]struct {
		ready int
		want  Verdict
	}{
		"previous still serves": {2, Failed},
		"nothing ready":         {0, Down},
	} {
		t.Run(name, func(t *testing.T) {
			w := newWorld(kube.KindDeployment, "api")
			w.status(2, tc.ready, 1, deadlineReason)
			w.change(t0, "", image("api:1", "api:2"))
			var b Book
			w.look(&b, t0.Add(11*time.Minute))
			r := b.Records[0]
			assert.Equal(t, tc.want, r.Verdict)
			assert.True(t, r.Stalled)
			assert.Equal(t, tc.ready > 0, r.OldServing)
		})
	}
}

func TestIncidentAboutTheWorkloadDecidesTheVerdict(t *testing.T) {
	for name, tc := range map[string]struct {
		reason string
		tier   incident.Tier
		want   Verdict
	}{
		"release regression": {reasons.ReleaseRegression,
			incident.Notify, Regressed},
		"crash loop told": {reasons.CrashLoopBackOff, incident.Page, Failed},
		"settled quietly": {reasons.CrashLoopBackOff, incident.Silent, Watching},
	} {
		t.Run(name, func(t *testing.T) {
			w := newWorld(kube.KindDeployment, "api")
			w.status(2, 2, 2, "")
			w.change(t0, "", image("api:1", "api:2"))
			w.incidents = []incident.Incident{{ID: "inc-1", Root: w.id,
				Tier: tc.tier, Opened: t0.Add(2 * time.Minute),
				Members: map[detection.Key]detection.Finding{
					{Entity: w.id, Reason: tc.reason}: {}}}}
			var b Book
			w.look(&b, t0.Add(3*time.Minute))
			assert.Equal(t, tc.want, b.Records[0].Verdict)
			if tc.want.Bad() {
				assert.Equal(t, "inc-1", b.Records[0].Incident)
			}
		})
	}
}

func TestNewerReleaseReplacesAnUnjudgedOne(t *testing.T) {
	w := newWorld(kube.KindDeployment, "api")
	w.status(2, 1, 1, "")
	w.change(t0, "", image("api:1", "api:2"))
	w.change(t0.Add(time.Minute), "", image("api:2", "api:3"))
	var b Book
	w.look(&b, t0.Add(2*time.Minute))
	require.Len(t, b.Records, 2)
	assert.Equal(t, Rolling, b.Records[0].Verdict)
	assert.Equal(t, Replaced, b.Records[1].Verdict)
	assert.True(t, b.Records[1].Final)
}

func TestOwnScopedAndLateReleases(t *testing.T) {
	w := newWorld(kube.KindDeployment, "watcher")
	w.status(1, 1, 1, "")
	w.change(t0, "", image("w:1", "w:2"))
	var b Book
	Observe(&b, View{Model: w.m, OwnNamespace: "shop",
		OwnWorkload: "watcher"}, t0.Add(time.Minute))
	assert.Empty(t, b.Records, "kwatch's own rollout")
	Observe(&b, View{Model: w.m, InScope: func(ns string) bool {
		return ns != "shop"
	}}, t0.Add(time.Minute))
	assert.Empty(t, b.Records, "outside the watched namespaces")
	Observe(&b, View{Model: w.m}, t0.Add(LateAfter+time.Minute))
	require.Len(t, b.Records, 1)
	assert.Equal(t, Unknown, b.Records[0].Verdict)
}

func TestStatefulSetAndDaemonSetRollouts(t *testing.T) {
	for _, kind := range []inventory.Kind{kube.KindStatefulSet,
		kube.KindDaemonSet} {
		t.Run(string(kind), func(t *testing.T) {
			w := newWorld(kind, "db")
			w.status(3, 3, 3, "")
			w.change(t0, "", inventory.FieldChange{
				Path: "containers[app].env.LEVEL", Before: "a", After: "b"})
			var b Book
			w.look(&b, t0.Add(time.Minute))
			w.look(&b, t0.Add(time.Minute+WatchWindow))
			require.Len(t, b.Records, 1)
			assert.Equal(t, Healthy, b.Records[0].Verdict)
			assert.Equal(t, []string{"environment"}, b.Records[0].Changed)
		})
	}
}

func TestRolloutThatNeverFinishesStalls(t *testing.T) {
	w := newWorld(kube.KindStatefulSet, "db")
	w.status(3, 3, 1, "")
	w.change(t0, "", image("db:1", "db:2"))
	var b Book
	w.look(&b, t0.Add(StallAfter-time.Minute))
	assert.Equal(t, Rolling, b.Records[0].Verdict)
	w.look(&b, t0.Add(StallAfter+time.Minute))
	assert.Equal(t, Failed, b.Records[0].Verdict)
	assert.True(t, b.Records[0].OldServing)
}
