package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// baselineRig is a Deployment with one pod and one container whose
// history the test fills.
type baselineRig struct {
	m         *inventory.Model
	dep       inventory.EntityID
	pod       inventory.EntityID
	container inventory.EntityID
}

func newBaselineRig(attrs map[string]inventory.Value) *baselineRig {
	r := &baselineRig{m: newTestModel(),
		dep: newID(kube.KindDeployment, "shop", "api"),
		pod: newID(kube.KindPod, "shop", "api-1-a")}
	rs := newID(kube.KindReplicaSet, "shop", "api-1")
	r.container = newID(kube.KindContainer, "shop", "api-1-a/app")
	put(r.m, r.dep, t0.Add(-48*time.Hour), nil)
	put(r.m, rs, t0.Add(-48*time.Hour), nil)
	put(r.m, r.pod, t0.Add(-time.Hour), map[string]inventory.Value{
		kube.AttrCreated: inventory.Time(t0.Add(-time.Hour)),
	})
	link(r.m, rs, inventory.OwnedBy, r.dep)
	link(r.m, r.pod, inventory.OwnedBy, rs)
	base := map[string]inventory.Value{
		kube.AttrState:        inventory.Text("running"),
		kube.AttrRestarts:     inventory.Number(6),
		kube.AttrLastFinished: inventory.Time(t0.Add(-time.Minute)),
	}
	for k, v := range attrs {
		base[k] = v
	}
	put(r.m, r.container, t0.Add(-time.Hour), base)
	link(r.m, r.container, inventory.PartOf, r.pod)
	return r
}

// learn gives the workload hours of restartsPerHour history ending an
// hour before t0, then the restarts of the last ten minutes.
func (r *baselineRig) learn(hours, perHour, now int) {
	b := r.m.Baselines()
	start := t0.Truncate(time.Hour).Add(-time.Duration(hours+1) * time.Hour)
	b.Touch(r.dep, start)
	for h := 1; h <= hours; h++ {
		b.AddRestarts(r.dep, perHour, start.Add(time.Duration(h)*time.Hour))
	}
	b.AddRestarts(r.dep, now, t0.Add(-10*time.Minute))
}

func (r *baselineRig) findings() []detection.Finding {
	return evaluate(Container{}, r.m, t0, r.container, nil).Findings
}

func TestRestartsFromAStableWorkloadAreUnusual(t *testing.T) {
	r := newBaselineRig(nil)
	r.learn(14, 0, 6)
	got := r.findings()
	require.Len(t, got, 1)
	assert.Equal(t, reasons.HighRestartCount, got[0].Reason)
	assert.Equal(t, detection.NormalUnusual, got[0].Normal)
	assert.Contains(t, got[0].Evidence, detection.Evidence{
		Label: detection.EvidenceBaseline,
		Value: "restarts 6×/h vs a usual 0×/h"})
}

func TestRestartsWithinTheWorkloadsNormalAreUsual(t *testing.T) {
	r := newBaselineRig(nil)
	r.learn(14, 2, 2)
	got := r.findings()
	require.Len(t, got, 1)
	assert.Equal(t, detection.NormalUsual, got[0].Normal)
	assert.Contains(t, got[0].Evidence, detection.Evidence{
		Label: detection.EvidenceBaseline,
		Value: "restarts 2×/h vs a usual 2×/h"})
}

func TestRestartsWithoutHistoryAreNotJudged(t *testing.T) {
	r := newBaselineRig(nil)
	r.learn(3, 0, 6)
	got := r.findings()
	require.Len(t, got, 1)
	assert.Equal(t, detection.NormalUnjudged, got[0].Normal)
	for _, e := range got[0].Evidence {
		assert.NotEqual(t, detection.EvidenceBaseline, e.Label)
	}
}

// A crash loop is never excused by history, however often it has
// happened before.
func TestCrashLoopIsNeverUsual(t *testing.T) {
	r := newBaselineRig(map[string]inventory.Value{
		kube.AttrState:       inventory.Text("waiting"),
		kube.AttrStateReason: inventory.Text(reasons.CrashLoopBackOff),
	})
	r.learn(14, 2, 2)
	got := r.findings()
	require.NotEmpty(t, got)
	for _, f := range got {
		assert.NotEqual(t, detection.NormalUsual, f.Normal, f.Reason)
	}
}

func TestOOMKillQuotesTheUsualMemoryPeak(t *testing.T) {
	r := newBaselineRig(map[string]inventory.Value{
		kube.AttrLastReason:    inventory.Text(reasons.OOMKilled),
		kube.AttrMemoryPeak24h: inventory.Number(900 << 20),
		kube.AttrMemoryLimit:   inventory.Number(1024 << 20),
		kube.AttrRestarts:      inventory.Number(6),
		kube.AttrLastFinished:  inventory.Time(t0.Add(-time.Minute)),
	})
	b := r.m.Baselines()
	hour := t0.Truncate(time.Hour).Add(-15 * time.Hour)
	for h := 0; h < 14; h++ {
		at := hour.Add(time.Duration(h) * time.Hour)
		b.AddMemory(r.dep, 300<<20, at)
		b.AddMemory(r.dep, 310<<20, at.Add(30*time.Minute))
	}
	got := r.findings()
	require.NotEmpty(t, got)
	var phrases []string
	for _, e := range got[0].Evidence {
		if e.Label == detection.EvidenceBaseline {
			phrases = append(phrases, e.Value)
		}
	}
	assert.Contains(t, phrases, "memory 900Mi vs a usual 310Mi peak")
	assert.NotEqual(t, detection.NormalUsual, got[0].Normal)
}

func TestMemoryNearTheLimitIsUsualForAWorkloadThatSitsThere(t *testing.T) {
	attrs := map[string]inventory.Value{
		kube.AttrMemoryRSS:   inventory.Number(940 << 20),
		kube.AttrMemoryLimit: inventory.Number(1024 << 20),
	}
	r := newBaselineRig(attrs)
	b := r.m.Baselines()
	hour := t0.Truncate(time.Hour).Add(-20 * time.Hour)
	for h := 0; h < 14; h++ {
		b.AddMemory(r.dep, 950<<20, hour.Add(time.Duration(h)*time.Hour))
	}
	b.AddMemory(r.dep, 950<<20, hour.Add(15*time.Hour))
	ctx := detection.Context{Model: r.m, Now: t0}
	e, _ := r.m.Entity(r.container)
	got := ContainerResources{}.Detect(ctx, e)
	require.Len(t, got, 1)
	assert.Equal(t, reasons.ContainerMemoryHigh, got[0].Reason)
	assert.Equal(t, detection.NormalUsual, got[0].Normal)
}

func TestSlowStartIsUsualForAWorkloadThatAlwaysStartsSlowly(t *testing.T) {
	r := newBaselineRig(nil)
	b := r.m.Baselines()
	for i := 0; i < 12; i++ {
		b.Add(r.dep, inventory.MetricReadySeconds, 240,
			t0.Add(-time.Duration(i+2)*time.Hour))
	}
	created := t0.Add(-4 * time.Minute)
	put(r.m, r.pod, created, map[string]inventory.Value{
		kube.AttrCreated:    inventory.Time(created),
		kube.AttrPhase:      inventory.Text("Running"),
		kube.AttrReady:      inventory.Bool(false),
		kube.AttrReadySince: inventory.Time(created),
	})
	got := evaluate(NewPod(PodThresholds{}), r.m, t0, r.pod, nil).Findings
	require.Len(t, got, 1)
	assert.Equal(t, reasons.ContainersNotReady, got[0].Reason)
	assert.Equal(t, detection.NormalUsual, got[0].Normal)
	assert.Contains(t, got[0].Evidence, detection.Evidence{
		Label: detection.EvidenceBaseline,
		Value: "readiness taking 4m vs a usual 4m"})
}

func TestSlowStartBeyondTheUsualIsUnusual(t *testing.T) {
	r := newBaselineRig(nil)
	b := r.m.Baselines()
	for i := 0; i < 12; i++ {
		b.Add(r.dep, inventory.MetricReadySeconds, 20,
			t0.Add(-time.Duration(i+2)*time.Hour))
	}
	created := t0.Add(-4 * time.Minute)
	put(r.m, r.pod, created, map[string]inventory.Value{
		kube.AttrCreated:    inventory.Time(created),
		kube.AttrPhase:      inventory.Text("Running"),
		kube.AttrReady:      inventory.Bool(false),
		kube.AttrReadySince: inventory.Time(created),
	})
	got := evaluate(NewPod(PodThresholds{}), r.m, t0, r.pod, nil).Findings
	require.Len(t, got, 1)
	assert.Equal(t, detection.NormalUnusual, got[0].Normal)
	assert.Contains(t, got[0].Evidence, detection.Evidence{
		Label: detection.EvidenceBaseline,
		Value: "readiness taking 4m vs a usual 20s"})
}

func TestAPodThatWasReadyIsNotJudgedByStartupTime(t *testing.T) {
	r := newBaselineRig(nil)
	b := r.m.Baselines()
	for i := 0; i < 12; i++ {
		b.Add(r.dep, inventory.MetricReadySeconds, 20,
			t0.Add(-time.Duration(i+2)*time.Hour))
	}
	created := t0.Add(-6 * time.Hour)
	put(r.m, r.pod, created, map[string]inventory.Value{
		kube.AttrCreated:    inventory.Time(created),
		kube.AttrPhase:      inventory.Text("Running"),
		kube.AttrReady:      inventory.Bool(false),
		kube.AttrReadySince: inventory.Time(t0.Add(-10 * time.Minute)),
	})
	got := evaluate(NewPod(PodThresholds{}), r.m, t0, r.pod, nil).Findings
	require.Len(t, got, 1)
	assert.Equal(t, detection.NormalUnjudged, got[0].Normal)
}
