package kube_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

var cvT0 = time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

func configEntity(t *testing.T, digests ...string) inventory.Entity {
	t.Helper()
	m := inventory.NewModel(inventory.Options{})
	id := inventory.CoreID(kube.KindConfigMap, "shop", "api-config")
	for i, d := range digests {
		m.Apply(inventory.Observation{Kind: inventory.Observed, Entity: id,
			At: cvT0.Add(time.Duration(i) * time.Minute),
			Attributes: map[string]inventory.Value{
				kube.AttrDataDigest: inventory.Text(d)}})
	}
	e, ok := m.Entity(id)
	assert.True(t, ok)
	return e
}

func TestConfigChangedAtIgnoresTheFirstDigest(t *testing.T) {
	_, ok := kube.ConfigChangedAt(configEntity(t, "a", "a"))
	assert.False(t, ok)
}

func TestConfigChangedAtIsWhenTheDigestChanged(t *testing.T) {
	at, ok := kube.ConfigChangedAt(configEntity(t, "a", "a", "b"))
	assert.True(t, ok)
	assert.Equal(t, cvT0.Add(2*time.Minute), at)
}

func podAt(created, started time.Time) inventory.Entity {
	attrs := map[string]inventory.Attribute{
		kube.AttrCreated: {Value: inventory.Time(created)}}
	if !started.IsZero() {
		attrs[kube.AttrContainersStarted] =
			inventory.Attribute{Value: inventory.Time(started)}
	}
	return inventory.Entity{Attributes: attrs}
}

func TestPodConfigAge(t *testing.T) {
	changed := cvT0
	tt := []struct {
		name string
		pod  inventory.Entity
		want kube.ConfigAge
	}{
		{"created after", podAt(cvT0.Add(time.Minute), time.Time{}),
			kube.ConfigAfter},
		{"never restarted", podAt(cvT0.Add(-time.Hour),
			cvT0.Add(-time.Hour)), kube.ConfigBefore},
		{"restarted since", podAt(cvT0.Add(-time.Hour),
			cvT0.Add(time.Minute)), kube.ConfigUnknown},
		{"not started", podAt(cvT0.Add(-time.Hour), time.Time{}),
			kube.ConfigUnknown},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, kube.PodConfigAge(tc.pod, changed))
		})
	}
}
