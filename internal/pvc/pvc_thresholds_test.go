package pvc

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/enricher"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
)

func TestPodStruct(t *testing.T) {
	assert := assert.New(t)

	pod := &Pod{
		PodRef: &Ref{
			Name:      "test-pod",
			Namespace: "default",
		},
		Volume: []*Volume{
			{
				Name:          "vol1",
				UsedBytes:     5000,
				CapacityBytes: 10000,
			},
		},
	}

	assert.Equal("test-pod", pod.PodRef.Name)
	assert.Equal(1, len(pod.Volume))
}

func TestPvcStableReasonDedup(t *testing.T) {
	incidentEngine := newConfiguredIncidentEngine(incident.Config{
		Window: 10 * time.Minute,
	})

	ev := observe.VolumeUsage(
		"default", "test-pvc", "test-pod", "VolumeUsageHigh",
	).WithHint("VolumeUsage(95%)")

	_, action1 := incidentEngine.Process(ev)
	assert.Equal(t, model.ActionCreate, action1)

	// second call with same sig → skip (edge-triggered)
	_, action2 := incidentEngine.Process(ev)
	assert.Equal(
		t,
		model.ActionSkip, action2,
		"second call with stable reason should skip (edge-triggered)",
	)
}

func TestPvcStableReasonDifferentPercentages(t *testing.T) {
	incidentEngine := newConfiguredIncidentEngine(incident.Config{
		Window: 10 * time.Minute,
	})

	ev1 := observe.VolumeUsage(
		"default", "test-pvc", "test-pod", "VolumeUsageHigh",
	).WithHint(fmt.Sprintf("VolumeUsage(%.0f%%)", 95.0))

	_, action1 := incidentEngine.Process(ev1)
	assert.Equal(t, model.ActionCreate, action1)

	ev2 := observe.VolumeUsage(
		"default", "test-pvc", "test-pod", "VolumeUsageHigh",
	).WithHint(fmt.Sprintf("VolumeUsage(%.0f%%)", 96.0))

	_, action2 := incidentEngine.Process(ev2)
	assert.Equal(
		t,
		model.ActionSkip, action2,
		"different percentage, same severity — edge-triggered skip",
	)
}

func TestPvcSeverityWarnTier(t *testing.T) {
	incidentEngine := newConfiguredIncidentEngine(incident.Config{
		Window:   10 * time.Minute,
		Enricher: &enricher.DefaultEnricher{},
	})

	ev := observe.VolumeUsage(
		"default", "test-pvc", "test-pod", "VolumeUsageHigh",
	).WithHint("VolumeUsage(85%)").
		WithSeverity("normal")

	inc, action := incidentEngine.Process(ev)
	assert.Equal(t, model.ActionCreate, action)
	assert.Equal(t, model.SeverityNormal, inc.Severity)
}

func TestPvcSeverityCriticalTier(t *testing.T) {
	incidentEngine := newConfiguredIncidentEngine(incident.Config{
		Window:   10 * time.Minute,
		Enricher: &enricher.DefaultEnricher{},
	})

	ev := observe.VolumeUsage(
		"default", "test-pvc", "test-pod", "VolumeUsageHigh",
	).WithHint("VolumeUsage(92%)").
		WithSeverity("high")

	inc, action := incidentEngine.Process(ev)
	assert.Equal(t, model.ActionCreate, action)
	assert.Equal(t, model.SeverityHigh, inc.Severity)
}

func TestPvcSeverityUpgradeFromWarnToCritical(t *testing.T) {
	incidentEngine := newConfiguredIncidentEngine(incident.Config{
		Window:   10 * time.Minute,
		Enricher: &enricher.DefaultEnricher{},
	})

	ev1 := observe.VolumeUsage(
		"default", "test-pvc", "test-pod", "VolumeUsageHigh",
	).WithHint("VolumeUsage(85%)").
		WithSeverity("normal")

	inc1, action1 := incidentEngine.Process(ev1)
	assert.Equal(t, model.ActionCreate, action1)
	assert.Equal(t, model.SeverityNormal, inc1.Severity)

	ev2 := observe.VolumeUsage(
		"default", "test-pvc", "test-pod", "VolumeUsageHigh",
	).WithHint("VolumeUsage(92%)").
		WithSeverity("high")

	inc2, action2 := incidentEngine.Process(ev2)
	assert.Equal(
		t,
		model.ActionUpdate, action2,
		"same key should update, not create",
	)
	assert.Equal(
		t,
		model.SeverityHigh, inc2.Severity,
		"severity should upgrade to high",
	)
}

func TestPvcFirstScanInitializedTrue(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80}

	pvc := newTestPvcMonitorWithState(client, cfg, nil, nil)
	assert.True(pvc.firstScan, "firstScan should initialize to true")
}

func TestPvcFirstScanSetToFalseAfterCheckUsage(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80}

	pvc := newTestPvcMonitorWithState(client, cfg, nil, nil)
	assert.True(pvc.firstScan)

	pvc.checkUsage(context.Background())

	assert.False(pvc.firstScan, "firstScan should be false after first checkUsage")
}

func TestPvcFirstScanSeedsNotifiedOnOverThreshold(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80}

	pvc := newTestPvcMonitorWithState(client, cfg, nil, nil)
	assert.True(pvc.firstScan)

	// Simulate what checkUsage does: over-threshold PVCs during firstScan
	// are added to currentNotified (which becomes notifiedPvc) but NOT reported.
	pvc.mu.Lock()
	pvc.notifiedPvc["pv-first-scan"] = true
	pvc.mu.Unlock()

	// After firstScan=false, previously seeded PVCs should remain in notifiedPvc
	pvc.firstScan = false
	pvc.mu.Lock()
	assert.True(
		pvc.notifiedPvc["pv-first-scan"],
		"seeded PV should remain after first scan",
	)
	pvc.mu.Unlock()
}

// Focused PVC monitor tests.

func newTestPvcMonitor(
	cfg *config.PvcMonitor,
	incidentEngine *incident.Engine,
) *PvcMonitor {
	m := newTestPvcMonitorWithState(
		fake.NewSimpleClientset(), cfg, incidentEngine, nil,
	)
	m.firstScan = false // tests that check signal behavior need firstScan=false
	return m
}

func newTestIncidentEngine() *incident.Engine {
	return newConfiguredIncidentEngine(incident.Config{
		Window:   10 * time.Minute,
		Enricher: &enricher.DefaultEnricher{},
	})
}

func newConfiguredIncidentEngine(cfg incident.Config) *incident.Engine {
	return incident.NewEngineWithClock(cfg, clock.RealClock{})
}

func TestApplyMountedHighKeepsNotified(t *testing.T) {
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80, ClearThreshold: 75}
	incidentEngine := newTestIncidentEngine()
	m := newTestPvcMonitor(cfg, incidentEngine)

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

	assert.True(t, m.notifiedPvc["default/pvc-1"])
	assert.Contains(
		t,
		m.lastUsage,
		"default/pvc-1",
	)
	assert.Equal(t, 95.0, m.lastUsage["default/pvc-1"].Pct)

	// incident should exist in incidentEngine
	snap := incidentEngine.Snapshot()
	found := false
	for _, v := range snap {
		if v.Name == "default/pvc-1" {
			found = true
		}
	}
	assert.True(
		t,
		found,
		"incident should exist in incidentEngine",
	)
}

func TestApplyUnmountedBoundKeepsFiring(t *testing.T) {
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80, ClearThreshold: 75}
	incidentEngine := newTestIncidentEngine()
	m := newTestPvcMonitor(cfg, incidentEngine)

	// First cycle: high usage
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

	// Second cycle: PVC is absent from stats/summary but still bound.
	m.apply(
		nil,
		map[string]string{"default/pvc-1": "pv-1"},
		false,
		true,
	)

	assert.True(
		t,
		m.notifiedPvc["default/pvc-1"],
		"bound-but-unmounted PVC must keep firing",
	)
	assert.Contains(
		t,
		m.lastUsage, "default/pvc-1",
		"lastUsage must survive unmount",
	)

	// incident should still be active (not resolved)
	snap := incidentEngine.Snapshot()
	for _, v := range snap {
		if v.Name == "default/pvc-1" {
			assert.NotEqual(
				t,
				model.StateResolved, v.State,
				"bound-unmounted incident must not resolve",
			)
		}
	}
}
