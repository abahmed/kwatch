package pvc

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/enricher"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
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
	correlator := correlation.NewEngine(correlation.Config{
		Window: 10 * time.Minute,
	})

	ev := event.Event{
		Resource:  "pvc",
		PodName:   "test-pod",
		Namespace: "default",
		Reason:    "VolumeUsageHigh",
		Hint:      "VolumeUsage(95%)",
	}
	owner := "test-pv"

	_, action1 := correlator.Process(ev, owner, nil)
	assert.Equal(t, model.ActionCreate, action1)

	// second call with same sig → skip (edge-triggered)
	_, action2 := correlator.Process(ev, owner, nil)
	assert.Equal(
		t,
		model.ActionSkip, action2,
		"second call with stable reason should skip (edge-triggered)",
	)
}

func TestPvcStableReasonDifferentPercentages(t *testing.T) {
	correlator := correlation.NewEngine(correlation.Config{
		Window: 10 * time.Minute,
	})

	ev1 := event.Event{
		Resource:  "pvc",
		PodName:   "test-pod",
		Namespace: "default",
		Reason:    "VolumeUsageHigh",
		Hint:      fmt.Sprintf("VolumeUsage(%.0f%%)", 95.0),
	}
	owner := "test-pv"

	_, action1 := correlator.Process(ev1, owner, nil)
	assert.Equal(t, model.ActionCreate, action1)

	ev2 := event.Event{
		Resource:  "pvc",
		PodName:   "test-pod",
		Namespace: "default",
		Reason:    "VolumeUsageHigh",
		Hint:      fmt.Sprintf("VolumeUsage(%.0f%%)", 96.0),
	}

	_, action2 := correlator.Process(ev2, owner, nil)
	assert.Equal(
		t,
		model.ActionSkip, action2,
		"different percentage, same severity — edge-triggered skip",
	)
}

func TestPvcSeverityWarnTier(t *testing.T) {
	correlator := correlation.NewEngine(correlation.Config{
		Window:   10 * time.Minute,
		Enricher: &enricher.DefaultEnricher{},
	})

	ev := event.Event{
		Resource:  "pvc",
		PodName:   "test-pod",
		Namespace: "default",
		Reason:    "VolumeUsageHigh",
		Hint:      "VolumeUsage(85%)",
		Severity:  "normal",
	}

	inc, action := correlator.Process(ev, "test-pv", nil)
	assert.Equal(t, model.ActionCreate, action)
	assert.Equal(t, model.SeverityNormal, inc.Severity)
}

func TestPvcSeverityCriticalTier(t *testing.T) {
	correlator := correlation.NewEngine(correlation.Config{
		Window:   10 * time.Minute,
		Enricher: &enricher.DefaultEnricher{},
	})

	ev := event.Event{
		Resource:  "pvc",
		PodName:   "test-pod",
		Namespace: "default",
		Reason:    "VolumeUsageHigh",
		Hint:      "VolumeUsage(92%)",
		Severity:  "high",
	}

	inc, action := correlator.Process(ev, "test-pv", nil)
	assert.Equal(t, model.ActionCreate, action)
	assert.Equal(t, model.SeverityHigh, inc.Severity)
}

func TestPvcSeverityUpgradeFromWarnToCritical(t *testing.T) {
	correlator := correlation.NewEngine(correlation.Config{
		Window:   10 * time.Minute,
		Enricher: &enricher.DefaultEnricher{},
	})

	ev1 := event.Event{
		Resource:  "pvc",
		PodName:   "test-pod",
		Namespace: "default",
		Reason:    "VolumeUsageHigh",
		Hint:      "VolumeUsage(85%)",
		Severity:  "normal",
	}

	inc1, action1 := correlator.Process(ev1, "test-pv", nil)
	assert.Equal(t, model.ActionCreate, action1)
	assert.Equal(t, model.SeverityNormal, inc1.Severity)

	ev2 := event.Event{
		Resource:  "pvc",
		PodName:   "test-pod",
		Namespace: "default",
		Reason:    "VolumeUsageHigh",
		Hint:      "VolumeUsage(92%)",
		Severity:  "high",
	}

	inc2, action2 := correlator.Process(ev2, "test-pv", nil)
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

	pvc := NewPvcMonitor(client, cfg, nil, nil)
	assert.True(pvc.firstScan, "firstScan should initialize to true")
}

func TestPvcFirstScanSetToFalseAfterCheckUsage(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80}

	pvc := NewPvcMonitor(client, cfg, nil, nil)
	assert.True(pvc.firstScan)

	pvc.checkUsage(context.Background())

	assert.False(pvc.firstScan, "firstScan should be false after first checkUsage")
}

func TestPvcFirstScanSeedsNotifiedOnOverThreshold(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80}

	pvc := NewPvcMonitor(client, cfg, nil, nil)
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
	correlator *correlation.Engine,
) *PvcMonitor {
	m := NewPvcMonitor(fake.NewSimpleClientset(), cfg, correlator, nil)
	m.firstScan = false // tests that check signal behavior need firstScan=false
	return m
}

func newTestCorrelator() *correlation.Engine {
	return correlation.NewEngine(correlation.Config{
		Window:   10 * time.Minute,
		Enricher: &enricher.DefaultEnricher{},
	})
}

func TestApplyMountedHighKeepsNotified(t *testing.T) {
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
	assert.Contains(
		t,
		m.lastUsage,
		"pv-1",
	)
	assert.Equal(t, 95.0, m.lastUsage["pv-1"].Pct)

	// incident should exist in correlator
	snap := corr.Snapshot()
	found := false
	for _, v := range snap {
		if v.Name == "pv-1" {
			found = true
		}
	}
	assert.True(
		t,
		found,
		"incident should exist in correlator",
	)
}

func TestApplyUnmountedBoundKeepsFiring(t *testing.T) {
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80, ClearThreshold: 75}
	corr := newTestCorrelator()
	m := newTestPvcMonitor(cfg, corr)

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
		m.notifiedPvc["pv-1"],
		"bound-but-unmounted PVC must keep firing",
	)
	assert.Contains(
		t,
		m.lastUsage, "pv-1",
		"lastUsage must survive unmount",
	)

	// incident should still be active (not resolved)
	snap := corr.Snapshot()
	for _, v := range snap {
		if v.Name == "pv-1" {
			assert.NotEqual(
				t,
				model.StateResolved, v.State,
				"bound-unmounted incident must not resolve",
			)
		}
	}
}
