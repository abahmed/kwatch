package pipeline

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

type samplerRig struct {
	t       *testing.T
	model   *inventory.Model
	sampler *baselineSampler
	deploy  inventory.EntityID
	pod     inventory.EntityID
}

func newSamplerRig(t *testing.T) *samplerRig {
	r := &samplerRig{
		t: t, model: inventory.NewModel(inventory.Options{}),
		deploy: inventory.CoreID(kube.KindDeployment, "shop", "api"),
		pod:    inventory.CoreID(kube.KindPod, "shop", "api-1-a"),
	}
	r.sampler = newBaselineSampler(r.model)
	rs := inventory.CoreID(kube.KindReplicaSet, "shop", "api-1")
	r.relate(r.pod, rs)
	r.relate(rs, r.deploy)
	return r
}

func (r *samplerRig) relate(from, to inventory.EntityID) {
	r.apply(inventory.Observation{Kind: inventory.Related,
		Entity: from, Relation: inventory.OwnedBy,
		Targets: []inventory.EntityID{to}})
}

func (r *samplerRig) apply(o inventory.Observation) {
	r.t.Helper()
	o.Source = "test"
	if o.At.IsZero() {
		o.At = timelineStart
	}
	_, err := r.model.Apply(o)
	require.NoError(r.t, err)
	r.sampler.observe(o)
}

func (r *samplerRig) observe(id inventory.EntityID,
	attrs map[string]inventory.Value) {
	r.apply(inventory.Observation{Kind: inventory.Observed, Entity: id,
		Attributes: attrs})
}

func (r *samplerRig) stat(metric inventory.Metric) inventory.Stat {
	stat, _ := r.model.Baselines().Stat(r.deploy, metric)
	return stat
}

func TestBaselineSamplerPodPendingAndReadyOnce(t *testing.T) {
	r := newSamplerRig(t)
	created := inventory.Time(timelineAt(0))
	r.observe(r.pod, map[string]inventory.Value{kube.AttrCreated: created})
	r.observe(r.pod, map[string]inventory.Value{
		kube.AttrCreated:   created,
		kube.AttrStartTime: inventory.Time(timelineAt(4 * time.Second)),
	})
	ready := map[string]inventory.Value{
		kube.AttrCreated:    created,
		kube.AttrStartTime:  inventory.Time(timelineAt(4 * time.Second)),
		kube.AttrReady:      inventory.Bool(true),
		kube.AttrReadySince: inventory.Time(timelineAt(20 * time.Second)),
	}
	r.observe(r.pod, ready)
	r.observe(r.pod, ready)

	assert.Equal(t, []float64{4}, r.stat(inventory.MetricPendingSeconds).Recent)
	assert.Equal(t, []float64{20}, r.stat(inventory.MetricReadySeconds).Recent)

	r.apply(inventory.Observation{Kind: inventory.Gone, Entity: r.pod})
	assert.NotContains(t, r.sampler.ready, r.pod)
}

func TestBaselineSamplerSkipsImplausibleDelays(t *testing.T) {
	r := newSamplerRig(t)
	r.observe(r.pod, map[string]inventory.Value{
		kube.AttrCreated:    inventory.Time(timelineAt(0)),
		kube.AttrReady:      inventory.Bool(true),
		kube.AttrReadySince: inventory.Time(timelineAt(3 * time.Hour)),
	})
	assert.Zero(t, r.stat(inventory.MetricReadySeconds).Count)
}

func TestBaselineSamplerCountsRestartDeltas(t *testing.T) {
	r := newSamplerRig(t)
	container := inventory.CoreID(kube.KindContainer, "shop", "api-1-a/app")
	r.apply(inventory.Observation{Kind: inventory.Related, Entity: container,
		Relation: inventory.PartOf, Targets: []inventory.EntityID{r.pod}})
	for _, restarts := range []float64{5, 5, 7} {
		r.observe(container, map[string]inventory.Value{
			kube.AttrRestarts: inventory.Number(restarts)})
	}
	r.apply(inventory.Observation{Kind: inventory.Observed,
		Entity: container, At: timelineAt(2 * time.Hour),
		Attributes: map[string]inventory.Value{
			kube.AttrRestarts: inventory.Number(8)}})
	assert.Equal(t, []float64{2, 0},
		r.stat(inventory.MetricRestartsPerHour).Recent)
}

func TestBaselineSamplerJobDurationPerCronJob(t *testing.T) {
	r := newSamplerRig(t)
	cron := inventory.CoreID(kube.KindCronJob, "shop", "report")
	job := inventory.CoreID(kube.KindJob, "shop", "report-1")
	r.apply(inventory.Observation{Kind: inventory.Related, Entity: job,
		Relation: inventory.OwnedBy, Targets: []inventory.EntityID{cron}})
	done := map[string]inventory.Value{
		kube.AttrStartTime: inventory.Time(timelineAt(0)),
		completeCondition:  inventory.Text("True"),
		completeCondition + kube.AttrConditionSince: inventory.Time(
			timelineAt(90 * time.Second)),
	}
	r.observe(job, map[string]inventory.Value{
		kube.AttrStartTime: inventory.Time(timelineAt(0))})
	r.observe(job, done)
	r.observe(job, done)
	stat, ok := r.model.Baselines().Stat(cron, inventory.MetricJobSeconds)
	require.True(t, ok)
	assert.Equal(t, []float64{90}, stat.Recent)
}
