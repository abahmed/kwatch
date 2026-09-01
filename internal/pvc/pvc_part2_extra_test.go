package pvc

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/model"
)

func TestApplyDeletedPvcResolves(t *testing.T) {
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80, ClearThreshold: 75}
	corr := newTestCorrelator()
	m := newTestPvcMonitor(cfg, corr)

	m.apply([]*PvcUsage{
		{
			Name:            "pvc-1",
			PVName:          "pv-1",
			Namespace:       "default",
			PodName:         "pod-1",
			UsagePercentage: 95,
		},
	}, map[string]string{
		"default/pvc-1": "pv-1",
		"default/pvc-2": "pv-2",
		"default/pvc-3": "pv-3",
	}, false, true)

	assert.True(t, m.notifiedPvc["pv-1"])

	// PVC deleted — empty pvByPVC
	m.apply(nil, map[string]string{}, false, true)

	assert.False(
		t,
		m.notifiedPvc["pv-1"],
		"deleted PVC must resolve",
	)
	assert.NotContains(
		t,
		m.lastUsage, "pv-1",
		"deleted PVC must be evicted from lastUsage",
	)

	// incident should now be resolved
	allResolved := true
	snap := corr.Snapshot()
	for _, v := range snap {
		if v.Name == "pv-1" {
			if v.State != model.StateResolved {
				allResolved = false
			}
		}
	}
	assert.True(
		t,
		allResolved,
		"deleted PVC incident must be resolved",
	)
}

func TestApplyRemountedBelowClearResolves(t *testing.T) {
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80, ClearThreshold: 75}
	corr := newTestCorrelator()
	m := newTestPvcMonitor(cfg, corr)

	// First: high usage
	m.apply([]*PvcUsage{
		{
			Name:            "pvc-1",
			PVName:          "pv-1",
			Namespace:       "default",
			PodName:         "pod-1",
			UsagePercentage: 95,
		},
	}, map[string]string{
		"default/pvc-1": "pv-1",
		"default/pvc-2": "pv-2",
		"default/pvc-3": "pv-3",
	}, false, true)

	assert.True(t, m.notifiedPvc["pv-1"])

	// Re-mounted but below clear threshold (e.g. 50%)
	m.apply([]*PvcUsage{
		{
			Name:            "pvc-1",
			PVName:          "pv-1",
			Namespace:       "default",
			PodName:         "pod-1",
			UsagePercentage: 50,
		},
	}, map[string]string{
		"default/pvc-1": "pv-1",
		"default/pvc-2": "pv-2",
		"default/pvc-3": "pv-3",
	}, false, true)

	assert.False(
		t,
		m.notifiedPvc["pv-1"],
		"re-mounted below clear must resolve",
	)
	assert.NotContains(
		t,
		m.lastUsage, "pv-1",
		"lastUsage must be evicted after genuine resolve",
	)
}

func TestApplyIncompleteSkipsClusterResolve(t *testing.T) {
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80, ClearThreshold: 75}
	corr := newTestCorrelator()
	m := newTestPvcMonitor(cfg, corr)

	// Full cycle: pv-1 fires
	m.apply([]*PvcUsage{
		{
			Name:            "pvc-1",
			PVName:          "pv-1",
			Namespace:       "default",
			PodName:         "pod-1",
			UsagePercentage: 95,
		},
	}, map[string]string{
		"default/pvc-1": "pv-1",
		"default/pvc-2": "pv-2",
		"default/pvc-3": "pv-3",
	}, false, true)

	assert.True(t, m.notifiedPvc["pv-1"])

	// Partial cycles skip the cluster-wide resolve pass.
	m.apply(
		nil,
		map[string]string{"default/pvc-1": "pv-1"},
		true,
		false,
	)

	assert.True(
		t,
		m.notifiedPvc["pv-1"],
		"incomplete cycle must not resolve bound PVs",
	)
}

func TestApplyFirstScanSuppressesSignal(t *testing.T) {
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80}
	corr := newTestCorrelator()
	m := newTestPvcMonitor(cfg, corr)
	m.firstScan = true

	// First scan with high usage: should add to notifiedPvc but NOT signal
	m.apply([]*PvcUsage{
		{
			Name:            "pvc-1",
			PVName:          "pv-1",
			Namespace:       "default",
			PodName:         "pod-1",
			UsagePercentage: 95,
		},
	}, map[string]string{
		"default/pvc-1": "pv-1",
		"default/pvc-2": "pv-2",
		"default/pvc-3": "pv-3",
	}, false, true)

	assert.True(t, m.notifiedPvc["pv-1"])
	assert.False(
		t,
		m.firstScan,
		"firstScan must be set to false after apply",
	)

	// No incident should exist in the correlator (firstScan suppressed the signal)
	snap := corr.Snapshot()
	for _, v := range snap {
		if v.Name == "pv-1" {
			t.Fatal("first scan should not create incidents in the correlator")
		}
	}
	assert.Equal(
		t,
		0, len(snap),
		"no incidents should exist after first scan",
	)
}
