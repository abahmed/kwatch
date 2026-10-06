package pipeline

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func (r *samplerRig) container() inventory.EntityID {
	c := inventory.CoreID(kube.KindContainer, "shop", "api-1-a/app")
	r.apply(inventory.Observation{Kind: inventory.Related, Entity: c,
		Relation: inventory.PartOf, Targets: []inventory.EntityID{r.pod}})
	return c
}

func TestBaselineSamplerLearnsQuietHoursFromTheWorkload(t *testing.T) {
	r := newSamplerRig(t)
	r.observe(r.deploy, map[string]inventory.Value{
		kube.AttrTemplateHash: inventory.Text("v1")})
	container := r.container()
	r.apply(inventory.Observation{Kind: inventory.Observed,
		Entity: container, At: timelineAt(13 * time.Hour),
		Attributes: map[string]inventory.Value{
			kube.AttrRestarts: inventory.Number(0)}})
	r.apply(inventory.Observation{Kind: inventory.Observed,
		Entity: r.deploy, At: timelineAt(13 * time.Hour),
		Attributes: map[string]inventory.Value{
			kube.AttrTemplateHash: inventory.Text("v1")}})
	stat := r.stat(inventory.MetricRestartsPerHour)
	assert.GreaterOrEqual(t, stat.Count, 12,
		"a workload that never restarted has a baseline of zeros")
	assert.Equal(t, 0.0, stat.High())
}

func TestBaselineSamplerRecordsMemoryPerHour(t *testing.T) {
	r := newSamplerRig(t)
	container := r.container()
	for i, bytes := range []float64{300 << 20, 500 << 20, 400 << 20} {
		r.apply(inventory.Observation{Kind: inventory.Observed,
			Entity: container,
			At:     timelineAt(time.Duration(i) * 20 * time.Minute),
			Attributes: map[string]inventory.Value{
				kube.AttrMemoryWorking: inventory.Number(bytes)}})
	}
	// The resident set wins when the kubelet reports both.
	r.apply(inventory.Observation{Kind: inventory.Observed,
		Entity: container, At: timelineAt(61 * time.Minute),
		Attributes: map[string]inventory.Value{
			kube.AttrMemoryWorking: inventory.Number(900 << 20),
			kube.AttrMemoryRSS:     inventory.Number(200 << 20)}})
	r.apply(inventory.Observation{Kind: inventory.Observed,
		Entity: container, At: timelineAt(2*time.Hour + time.Minute),
		Attributes: map[string]inventory.Value{
			kube.AttrMemoryRSS: inventory.Number(100 << 20)}})
	assert.Equal(t, []float64{500 << 20, 200 << 20},
		r.stat(inventory.MetricMemoryPeak).Recent)
}

func TestBaselineSamplerNewTemplateStartsMemoryAgain(t *testing.T) {
	r := newSamplerRig(t)
	container := r.container()
	template := func(hash string, at time.Duration) {
		r.apply(inventory.Observation{Kind: inventory.Observed,
			Entity: r.deploy, At: timelineAt(at),
			Attributes: map[string]inventory.Value{
				kube.AttrTemplateHash: inventory.Text(hash)}})
	}
	template("v1", 0)
	for h := 0; h < 3; h++ {
		r.apply(inventory.Observation{Kind: inventory.Observed,
			Entity: container, At: timelineAt(time.Duration(h) * time.Hour),
			Attributes: map[string]inventory.Value{
				kube.AttrMemoryRSS: inventory.Number(300 << 20)}})
	}
	require.NotZero(t, r.stat(inventory.MetricMemoryPeak).Count)
	template("v2", 3*time.Hour)
	assert.Zero(t, r.stat(inventory.MetricMemoryPeak).Count)
}

func TestBaselineSamplerCountsWarningEventsPerReason(t *testing.T) {
	r := newSamplerRig(t)
	note := func(reason string, count int, at time.Duration) {
		r.apply(inventory.Observation{Kind: inventory.Noted, Entity: r.pod,
			At: timelineAt(at), Note: inventory.Note{
				Reason: reason, Warning: true, Count: count,
				At: timelineAt(at)}})
	}
	note("BackOff", 1, 0)
	note("BackOff", 4, 10*time.Minute)
	note("Unhealthy", 1, 20*time.Minute)
	note("Normal", 0, 25*time.Minute)
	note("BackOff", 4, 2*time.Hour)
	assert.Equal(t, []float64{4, 0},
		r.stat(inventory.WarningMetric("BackOff")).Recent,
		"a repeat adds only its new occurrences; the next hour was quiet")
	assert.Equal(t, []float64{1, 0},
		r.stat(inventory.WarningMetric("Unhealthy")).Recent)
}

func TestBaselineSamplerIgnoresNormalEvents(t *testing.T) {
	r := newSamplerRig(t)
	r.apply(inventory.Observation{Kind: inventory.Noted, Entity: r.pod,
		Note: inventory.Note{Reason: "Pulled", Count: 1}})
	_, ok := r.model.Baselines().Stat(r.deploy,
		inventory.WarningMetric("Pulled"))
	assert.False(t, ok)
}
