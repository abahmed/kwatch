package pvc

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/alert"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/enricher"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/state"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNewPvcMonitor(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.PvcMonitor{
		Enabled:   true,
		Threshold: 80,
		Interval:  5,
	}
	alertMgr := &alert.AlertManager{}

	pvc := NewPvcMonitor(client, cfg, alertMgr, nil, nil)
	assert.NotNil(pvc)
	assert.Equal(client, pvc.client)
	assert.Equal(cfg, pvc.config)
	assert.Equal(alertMgr, pvc.alertManager)
	assert.NotNil(pvc.notifiedPvc)
}

func TestNewPvcMonitorNilConfig(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	alertMgr := &alert.AlertManager{}

	pvc := NewPvcMonitor(client, nil, alertMgr, nil, nil)
	assert.NotNil(pvc)
	assert.Nil(pvc.config)
}

func TestNewPvcMonitorNilAlertManager(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.PvcMonitor{}

	pvc := NewPvcMonitor(client, cfg, nil, nil, nil)
	assert.NotNil(pvc)
	assert.Nil(pvc.alertManager)
}

func TestStartDisabled(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.PvcMonitor{
		Enabled: false,
	}
	alertMgr := &alert.AlertManager{}

	pvc := NewPvcMonitor(client, cfg, alertMgr, nil, nil)
	pvc.Start(context.Background())
}

func TestCleanupUnderThreshold(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.PvcMonitor{}
	alertMgr := &alert.AlertManager{}

	pvc := NewPvcMonitor(client, cfg, alertMgr, nil, nil)

	for i := 0; i < 100; i++ {
		pvc.notifiedPvc[string(rune(i))] = true
	}

	initialCount := len(pvc.notifiedPvc)
	pvc.cleanup()
	assert.Equal(initialCount, len(pvc.notifiedPvc))
}

func TestCleanupOverThreshold(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.PvcMonitor{}
	alertMgr := &alert.AlertManager{}

	pvc := NewPvcMonitor(client, cfg, alertMgr, nil, nil)

	for i := 0; i < 1001; i++ {
		pvc.notifiedPvc[string(rune(i))] = true
	}

	pvc.cleanup()
	assert.Equal(0, len(pvc.notifiedPvc))
}

func TestCleanupExactlyThreshold(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.PvcMonitor{}
	alertMgr := &alert.AlertManager{}

	pvc := NewPvcMonitor(client, cfg, alertMgr, nil, nil)

	for i := 0; i < 1000; i++ {
		pvc.notifiedPvc[string(rune(i))] = true
	}

	pvc.cleanup()
	assert.Equal(1000, len(pvc.notifiedPvc))
}

func TestPvcMonitorConcurrency(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.PvcMonitor{}
	alertMgr := &alert.AlertManager{}

	pvc := NewPvcMonitor(client, cfg, alertMgr, nil, nil)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pvc.mu.Lock()
			pvc.notifiedPvc["key"] = true
			pvc.mu.Unlock()
		}()
	}
	wg.Wait()
}

func TestPvcUsageStruct(t *testing.T) {
	assert := assert.New(t)

	usage := &PvcUsage{
		Name:            "test-pvc",
		PVName:          "test-pv",
		Namespace:       "default",
		PodName:         "test-pod",
		UsagePercentage: 85.5,
	}

	assert.Equal("test-pvc", usage.Name)
	assert.Equal("test-pv", usage.PVName)
	assert.Equal("default", usage.Namespace)
	assert.Equal("test-pod", usage.PodName)
	assert.Equal(85.5, usage.UsagePercentage)
}

func TestSummaryResponseUnmarshal(t *testing.T) {
	assert := assert.New(t)

	jsonData := `{
		"pods": [
			{
				"podRef": {"name": "pod1", "namespace": "default"},
				"volume": [
					{
						"usedBytes": 8500,
						"capacityBytes": 10000,
						"name": "vol1",
						"pvcRef": {"name": "pvc1", "namespace": "default"}
					}
				]
			}
		]
	}`

	var summary SummaryResponse
	err := json.Unmarshal([]byte(jsonData), &summary)
	assert.Nil(err)
	assert.Equal(1, len(summary.Pods))
	assert.Equal("pod1", summary.Pods[0].PodRef.Name)
	assert.Equal(85.0, (float64(summary.Pods[0].Volume[0].UsedBytes)/float64(summary.Pods[0].Volume[0].CapacityBytes))*100)
}

func TestSummaryResponseEmptyVolumes(t *testing.T) {
	assert := assert.New(t)

	jsonData := `{
		"pods": [
			{
				"podRef": {"name": "pod1", "namespace": "default"},
				"volume": []
			}
		]
	}`

	var summary SummaryResponse
	err := json.Unmarshal([]byte(jsonData), &summary)
	assert.Nil(err)
	assert.Equal(1, len(summary.Pods))
	assert.Equal(0, len(summary.Pods[0].Volume))
}

func TestSummaryResponseNilPvcRef(t *testing.T) {
	assert := assert.New(t)

	jsonData := `{
		"pods": [
			{
				"podRef": {"name": "pod1", "namespace": "default"},
				"volume": [
					{
						"usedBytes": 5000,
						"capacityBytes": 10000,
						"name": "vol1"
					}
				]
			}
		]
	}`

	var summary SummaryResponse
	err := json.Unmarshal([]byte(jsonData), &summary)
	assert.Nil(err)
	assert.Equal(1, len(summary.Pods))
	assert.Nil(summary.Pods[0].Volume[0].PvcRef)
}

func TestSummaryResponseEmptyPvcRefName(t *testing.T) {
	assert := assert.New(t)

	jsonData := `{
		"pods": [
			{
				"podRef": {"name": "pod1", "namespace": "default"},
				"volume": [
					{
						"usedBytes": 5000,
						"capacityBytes": 10000,
						"name": "vol1",
						"pvcRef": {"name": "", "namespace": "default"}
					}
				]
			}
		]
	}`

	var summary SummaryResponse
	err := json.Unmarshal([]byte(jsonData), &summary)
	assert.Nil(err)
	assert.Equal("", summary.Pods[0].Volume[0].PvcRef.Name)
}

func TestCheckUsageNoNodes(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.PvcMonitor{
		Enabled:   true,
		Threshold: 80,
	}
	alertMgr := &alert.AlertManager{}

	pvc := NewPvcMonitor(client, cfg, alertMgr, nil, nil)
	pvc.checkUsage(context.Background())

	assert.Equal(0, len(pvc.notifiedPvc))
}

func TestCheckUsageAlreadyNotified(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.PvcMonitor{
		Enabled:   true,
		Threshold: 80,
	}
	alertMgr := &alert.AlertManager{}

	pvc := NewPvcMonitor(client, cfg, alertMgr, nil, nil)
	pvc.notifiedPvc["existing-pv"] = true

	assert.True(pvc.notifiedPvc["existing-pv"])
}

func TestCheckUsageUnderThreshold(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.PvcMonitor{
		Enabled:   true,
		Threshold: 90,
	}
	alertMgr := &alert.AlertManager{}

	pvc := NewPvcMonitor(client, cfg, alertMgr, nil, nil)

	usage := &PvcUsage{
		Name:            "test-pvc",
		PVName:          "test-pv",
		Namespace:       "default",
		PodName:         "test-pod",
		UsagePercentage: 50.0,
	}

	if usage.UsagePercentage >= cfg.Threshold {
		pvc.notifiedPvc[usage.PVName] = true
	}

	assert.Equal(0, len(pvc.notifiedPvc))
}

func TestCheckUsageOverThreshold(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.PvcMonitor{
		Enabled:   true,
		Threshold: 80,
	}
	alertMgr := &alert.AlertManager{}

	pvc := NewPvcMonitor(client, cfg, alertMgr, nil, nil)

	usage := &PvcUsage{
		Name:            "test-pvc",
		PVName:          "test-pv",
		Namespace:       "default",
		PodName:         "test-pod",
		UsagePercentage: 95.0,
	}

	if usage.UsagePercentage >= cfg.Threshold {
		pvc.notifiedPvc[usage.PVName] = true
	}

	assert.Equal(1, len(pvc.notifiedPvc))
	assert.True(pvc.notifiedPvc["test-pv"])
}

func TestRefStruct(t *testing.T) {
	assert := assert.New(t)

	ref := &Ref{
		Name:      "test-name",
		Namespace: "test-namespace",
	}

	assert.Equal("test-name", ref.Name)
	assert.Equal("test-namespace", ref.Namespace)
}

func TestVolumeStruct(t *testing.T) {
	assert := assert.New(t)

	volume := &Volume{
		UsedBytes:     5000,
		CapacityBytes: 10000,
		Name:          "test-volume",
		PvcRef: &Ref{
			Name:      "test-pvc",
			Namespace: "default",
		},
	}

	assert.Equal(int64(5000), volume.UsedBytes)
	assert.Equal(int64(10000), volume.CapacityBytes)
	assert.Equal("test-volume", volume.Name)
	assert.Equal("test-pvc", volume.PvcRef.Name)
}

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
	assert.Equal(t, model.ActionSkip, action2, "second call with stable reason should skip (edge-triggered)")
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
	assert.Equal(t, model.ActionSkip, action2, "different percentage, same severity — edge-triggered skip")
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
	assert.Equal(t, "normal", inc.Severity)
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
	assert.Equal(t, "high", inc.Severity)
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
	assert.Equal(t, "normal", inc1.Severity)

	ev2 := event.Event{
		Resource:  "pvc",
		PodName:   "test-pod",
		Namespace: "default",
		Reason:    "VolumeUsageHigh",
		Hint:      "VolumeUsage(92%)",
		Severity:  "high",
	}

	inc2, action2 := correlator.Process(ev2, "test-pv", nil)
	assert.Equal(t, model.ActionUpdate, action2, "same key should update, not create")
	assert.Equal(t, "high", inc2.Severity, "severity should upgrade to high")
}

func TestPvcFirstScanInitializedTrue(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80}
	alertMgr := &alert.AlertManager{}

	pvc := NewPvcMonitor(client, cfg, alertMgr, nil, nil)
	assert.True(pvc.firstScan, "firstScan should initialize to true")
}

func TestPvcFirstScanSetToFalseAfterCheckUsage(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80}
	alertMgr := &alert.AlertManager{}

	pvc := NewPvcMonitor(client, cfg, alertMgr, nil, nil)
	assert.True(pvc.firstScan)

	pvc.checkUsage(context.Background())

	assert.False(pvc.firstScan, "firstScan should be false after first checkUsage")
}

func TestPvcFirstScanSeedsNotifiedOnOverThreshold(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80}
	alertMgr := &alert.AlertManager{}

	pvc := NewPvcMonitor(client, cfg, alertMgr, nil, nil)
	assert.True(pvc.firstScan)

	// Simulate what checkUsage does: over-threshold PVCs during firstScan
	// are added to currentNotified (which becomes notifiedPvc) but NOT reported.
	pvc.mu.Lock()
	pvc.notifiedPvc["pv-first-scan"] = true
	pvc.mu.Unlock()

	// After firstScan=false, previously seeded PVCs should remain in notifiedPvc
	pvc.firstScan = false
	pvc.mu.Lock()
	assert.True(pvc.notifiedPvc["pv-first-scan"], "seeded PV should remain in notifiedPvc after first scan")
	pvc.mu.Unlock()
}

// ── apply() unit tests ────────────────────────────────────────

func newTestPvcMonitor(cfg *config.PvcMonitor, correlator *correlation.Engine) *PvcMonitor {
	m := NewPvcMonitor(fake.NewSimpleClientset(), cfg, &alert.AlertManager{}, correlator, nil)
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
		{Name: "pvc-1", PVName: "pv-1", Namespace: "default", PodName: "pod-1", UsagePercentage: 95},
	}, map[string]string{"default/pvc-1": "pv-1"}, false, true)

	assert.True(t, m.notifiedPvc["pv-1"])
	assert.Contains(t, m.lastUsage, "pv-1")
	assert.Equal(t, 95.0, m.lastUsage["pv-1"].Pct)

	// incident should exist in correlator
	snap := corr.Snapshot()
	found := false
	for _, v := range snap {
		if v.Name == "pv-1" {
			found = true
		}
	}
	assert.True(t, found, "incident should exist in correlator")
}

func TestApplyUnmountedBoundKeepsFiring(t *testing.T) {
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80, ClearThreshold: 75}
	corr := newTestCorrelator()
	m := newTestPvcMonitor(cfg, corr)

	// First cycle: high usage
	m.apply([]*PvcUsage{
		{Name: "pvc-1", PVName: "pv-1", Namespace: "default", PodName: "pod-1", UsagePercentage: 95},
	}, map[string]string{"default/pvc-1": "pv-1"}, false, true)

	// Second cycle: PVC is absent from stats/summary (unmounted) but still bound
	m.apply(nil, map[string]string{"default/pvc-1": "pv-1"}, false, true)

	assert.True(t, m.notifiedPvc["pv-1"], "bound-but-unmounted PVC must keep firing")
	assert.Contains(t, m.lastUsage, "pv-1", "lastUsage must survive unmount")

	// incident should still be active (not resolved)
	snap := corr.Snapshot()
	for _, v := range snap {
		if v.Name == "pv-1" {
			assert.NotEqual(t, model.StateResolved, v.State, "bound-unmounted incident must not resolve")
		}
	}
}

func TestApplyDeletedPvcResolves(t *testing.T) {
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80, ClearThreshold: 75}
	corr := newTestCorrelator()
	m := newTestPvcMonitor(cfg, corr)

	m.apply([]*PvcUsage{
		{Name: "pvc-1", PVName: "pv-1", Namespace: "default", PodName: "pod-1", UsagePercentage: 95},
	}, map[string]string{"default/pvc-1": "pv-1"}, false, true)

	assert.True(t, m.notifiedPvc["pv-1"])

	// PVC deleted — empty pvByPVC
	m.apply(nil, map[string]string{}, false, true)

	assert.False(t, m.notifiedPvc["pv-1"], "deleted PVC must resolve")
	assert.NotContains(t, m.lastUsage, "pv-1", "deleted PVC must be evicted from lastUsage")

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
	assert.True(t, allResolved, "deleted PVC incident must be resolved")
}

func TestApplyRemountedBelowClearResolves(t *testing.T) {
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80, ClearThreshold: 75}
	corr := newTestCorrelator()
	m := newTestPvcMonitor(cfg, corr)

	// First: high usage
	m.apply([]*PvcUsage{
		{Name: "pvc-1", PVName: "pv-1", Namespace: "default", PodName: "pod-1", UsagePercentage: 95},
	}, map[string]string{"default/pvc-1": "pv-1"}, false, true)

	assert.True(t, m.notifiedPvc["pv-1"])

	// Re-mounted but below clear threshold (e.g. 50%)
	m.apply([]*PvcUsage{
		{Name: "pvc-1", PVName: "pv-1", Namespace: "default", PodName: "pod-1", UsagePercentage: 50},
	}, map[string]string{"default/pvc-1": "pv-1"}, false, true)

	assert.False(t, m.notifiedPvc["pv-1"], "re-mounted below clear must resolve")
	assert.NotContains(t, m.lastUsage, "pv-1", "lastUsage must be evicted after genuine resolve")
}

func TestApplyIncompleteSkipsClusterResolve(t *testing.T) {
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80, ClearThreshold: 75}
	corr := newTestCorrelator()
	m := newTestPvcMonitor(cfg, corr)

	// Full cycle: pv-1 fires
	m.apply([]*PvcUsage{
		{Name: "pvc-1", PVName: "pv-1", Namespace: "default", PodName: "pod-1", UsagePercentage: 95},
	}, map[string]string{"default/pvc-1": "pv-1"}, false, true)

	assert.True(t, m.notifiedPvc["pv-1"])

	// Partial cycle (incomplete=true): pv-1 is absent but still bound →
	// must NOT resolve (cluster-wide resolve pass is skipped)
	m.apply(nil, map[string]string{"default/pvc-1": "pv-1"}, true, false)

	assert.True(t, m.notifiedPvc["pv-1"], "incomplete cycle must not resolve bound PVs")
}

func TestApplyFirstScanSuppressesSignal(t *testing.T) {
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80}
	corr := newTestCorrelator()
	m := newTestPvcMonitor(cfg, corr)
	m.firstScan = true

	// First scan with high usage: should add to notifiedPvc but NOT signal
	m.apply([]*PvcUsage{
		{Name: "pvc-1", PVName: "pv-1", Namespace: "default", PodName: "pod-1", UsagePercentage: 95},
	}, map[string]string{"default/pvc-1": "pv-1"}, false, true)

	assert.True(t, m.notifiedPvc["pv-1"])
	assert.False(t, m.firstScan, "firstScan must be set to false after apply")

	// No incident should exist in the correlator (firstScan suppressed the signal)
	snap := corr.Snapshot()
	for _, v := range snap {
		if v.Name == "pv-1" {
			t.Fatal("first scan should not create incidents in the correlator")
		}
	}
	assert.Equal(t, 0, len(snap), "no incidents should exist after first scan")
}

// ── SampleNode / persist tests ────────────────────────────────

func TestSampleNodeDebounce(t *testing.T) {
	ctx := context.Background()
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80}
	corr := newTestCorrelator()
	m := newTestPvcMonitor(cfg, corr)
	m.getNodeUsageFn = func(_ context.Context, _ string, _ map[string]string) ([]*PvcUsage, error) {
		return nil, nil
	}

	// Create a fake Ready node so SampleNode doesn't bail early
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
		Status: v1.NodeStatus{
			Conditions: []v1.NodeCondition{{Type: v1.NodeReady, Status: v1.ConditionTrue}},
		},
	}
	_, err := m.client.CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
	assert.Nil(t, err)

	m.SampleNode(ctx, "node-1")
	// Second call within debounce window should be skipped
	// We can verify by checking lastNodeSample timestamp
	m.mu.RLock()
	_, exists := m.lastNodeSample["node-1"]
	m.mu.RUnlock()
	assert.True(t, exists, "node-1 should be tracked after first SampleNode")

	// Call again — should hit debounce, not panic
	m.SampleNode(ctx, "node-1")
}

func TestSampleNodeEmptyNodeName(t *testing.T) {
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80}
	corr := newTestCorrelator()
	m := newTestPvcMonitor(cfg, corr)

	m.SampleNode(context.Background(), "")
	// no panic, no entry
	assert.Nil(t, m.lastNodeSample)
}

func TestSampleNodeDisabled(t *testing.T) {
	cfg := &config.PvcMonitor{Enabled: false}
	corr := newTestCorrelator()
	m := newTestPvcMonitor(cfg, corr)

	m.SampleNode(context.Background(), "node-1")
	assert.Nil(t, m.lastNodeSample)
}

func TestPersistWritesToConfigMap(t *testing.T) {
	client := fake.NewSimpleClientset()
	sm := state.NewStateManager(client, "kwatch")

	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80}
	corr := newTestCorrelator()
	m := NewPvcMonitor(client, cfg, &alert.AlertManager{}, corr, sm)

	// Populate lastUsage via apply
	m.apply([]*PvcUsage{
		{Name: "pvc-1", PVName: "pv-1", Namespace: "default", PodName: "pod-1", UsagePercentage: 95},
	}, map[string]string{"default/pvc-1": "pv-1"}, false, true)

	// Persist
	m.persist(context.Background())

	// Verify the ConfigMap was written
	cm, err := client.CoreV1().ConfigMaps("kwatch").Get(context.Background(), "kwatch-pvc", metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, cm)
	assert.NotEmpty(t, cm.BinaryData["pvc-usage"])

	// Verify we can read it back
	loaded := sm.GetPvcUsage(context.Background())
	assert.NotNil(t, loaded)
	assert.Equal(t, 95.0, loaded["pv-1"].Pct)
	assert.Equal(t, "default", loaded["pv-1"].Namespace)
	assert.Equal(t, "pvc-1", loaded["pv-1"].Name)
}

func TestPersistNilStateDoesNothing(t *testing.T) {
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80}
	corr := newTestCorrelator()
	m := newTestPvcMonitor(cfg, corr) // state == nil

	m.apply([]*PvcUsage{
		{Name: "pvc-1", PVName: "pv-1", Namespace: "default", PodName: "pod-1", UsagePercentage: 95},
	}, map[string]string{"default/pvc-1": "pv-1"}, false, true)

	m.persist(context.Background())
	// no panic, no ConfigMap written (verified by the lack of error)
}

func TestPersistOnlyOnSweepNotOnSampleNode(t *testing.T) {
	client := fake.NewSimpleClientset()
	sm := state.NewStateManager(client, "kwatch")

	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80}
	corr := newTestCorrelator()
	m := NewPvcMonitor(client, cfg, &alert.AlertManager{}, corr, sm)

	// Apply some data
	m.apply([]*PvcUsage{
		{Name: "pvc-1", PVName: "pv-1", Namespace: "default", PodName: "pod-1", UsagePercentage: 95},
	}, map[string]string{"default/pvc-1": "pv-1"}, false, true)

	// Apply again (simulating SampleNode calling apply with incomplete=true)
	m.apply([]*PvcUsage{
		{Name: "pvc-1", PVName: "pv-1", Namespace: "default", PodName: "pod-1", UsagePercentage: 96},
	}, map[string]string{"default/pvc-1": "pv-1"}, true, false)

	// No persist called yet — ConfigMap should not exist
	_, err := client.CoreV1().ConfigMaps("kwatch").Get(context.Background(), "kwatch-pvc", metav1.GetOptions{})
	assert.True(t, len(err.Error()) > 0, "ConfigMap should not exist before persist")

	// Now persist (only the sweep does this)
	m.persist(context.Background())

	cm, err := client.CoreV1().ConfigMaps("kwatch").Get(context.Background(), "kwatch-pvc", metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, cm)
}

// ── Persistence round-trip (restart survival) ─────────────────

func TestPersistenceRoundTripRestoresIncidents(t *testing.T) {
	client := fake.NewSimpleClientset()
	sm := state.NewStateManager(client, "kwatch")

	// Pre-populate the ConfigMap with a high PVC sample (simulates prior run)
	preExisting := map[string]state.PvcSample{
		"pv-unmounted": {Pct: 95, Namespace: "default", Name: "pvc-backup", Seen: time.Now()},
		"pv-low":       {Pct: 50, Namespace: "default", Name: "pvc-low", Seen: time.Now()},
	}
	err := sm.SavePvcUsage(context.Background(), preExisting)
	assert.Nil(t, err)

	// Create a PvcMonitor with the SAME state manager (simulates restart)
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80, CriticalThreshold: 90}
	corr := newTestCorrelator()
	m := NewPvcMonitor(client, cfg, &alert.AlertManager{}, corr, sm)

	// Simulate Start's seed block (which runs before first checkUsage)
	if seed := sm.GetPvcUsage(context.Background()); seed != nil {
		m.mu.Lock()
		m.lastUsage = seed
		for pv, s := range seed {
			if s.Pct >= cfg.Threshold {
				m.notifiedPvc[pv] = true
			}
		}
		m.mu.Unlock()
	}

	// pv-unmounted (95%) should be in notifiedPvc; pv-low (50%) should not
	assert.True(t, m.notifiedPvc["pv-unmounted"], "high PVC must be re-notified after restart")
	assert.False(t, m.notifiedPvc["pv-low"], "low PVC must not be notified")
	assert.Contains(t, m.lastUsage, "pv-unmounted")
	assert.Contains(t, m.lastUsage, "pv-low")

	// Verify lastUsage values are correct
	assert.Equal(t, 95.0, m.lastUsage["pv-unmounted"].Pct)
	assert.Equal(t, "pvc-backup", m.lastUsage["pv-unmounted"].Name)
}

func TestPersistenceRoundTripKeepFiringWithoutRemount(t *testing.T) {
	client := fake.NewSimpleClientset()
	sm := state.NewStateManager(client, "kwatch")

	// Pre-populate ConfigMap with a high PVC sample
	preExisting := map[string]state.PvcSample{
		"pv-backup": {Pct: 95, Namespace: "default", Name: "pvc-backup", Seen: time.Now()},
	}
	err := sm.SavePvcUsage(context.Background(), preExisting)
	assert.Nil(t, err)

	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80, ClearThreshold: 75}
	corr := newTestCorrelator()
	m := NewPvcMonitor(client, cfg, &alert.AlertManager{}, corr, sm)

	// Seed from configmap (same as Start)
	if seed := sm.GetPvcUsage(context.Background()); seed != nil {
		m.mu.Lock()
		m.lastUsage = seed
		for pv, s := range seed {
			if s.Pct >= cfg.Threshold {
				m.notifiedPvc[pv] = true
			}
		}
		m.mu.Unlock()
	}

	assert.True(t, m.notifiedPvc["pv-backup"])

	// Run a full apply cycle where pv-backup is NOT in pvcUsages (it's unmounted)
	// but still in pvByPVC (bound). It should KEEP firing.
	m.apply(nil, map[string]string{"default/pvc-backup": "pv-backup"}, false, true)

	assert.True(t, m.notifiedPvc["pv-backup"],
		"unmounted high PVC from persisted state must keep firing after apply")
	assert.Contains(t, m.lastUsage, "pv-backup",
		"lastUsage must survive the apply cycle")
}

func TestPersistenceRoundTripNoPreviousState(t *testing.T) {
	client := fake.NewSimpleClientset()
	sm := state.NewStateManager(client, "kwatch")

	// No pre-existing ConfigMap
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80}
	corr := newTestCorrelator()
	m := NewPvcMonitor(client, cfg, &alert.AlertManager{}, corr, sm)

	// Seed from ConfigMap (should return nil since there's no data)
	assert.Nil(t, sm.GetPvcUsage(context.Background()))

	// Verify nothing was seeded
	assert.Equal(t, 0, len(m.lastUsage))
	assert.Equal(t, 0, len(m.notifiedPvc))
}

// ── Persist write-count test ──────────────────────────────────

func TestPersistWriteCountSampleNodeDoesNotPersist(t *testing.T) {
	client := fake.NewSimpleClientset()
	sm := state.NewStateManager(client, "kwatch")

	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80, ClearThreshold: 75}
	corr := newTestCorrelator()
	m := NewPvcMonitor(client, cfg, &alert.AlertManager{}, corr, sm)

	// Apply data (simulating what SampleNode does — incomplete=true)
	// SampleNode calls apply with incomplete=true, which should NOT persist
	m.apply([]*PvcUsage{
		{Name: "pvc-1", PVName: "pv-1", Namespace: "default", PodName: "pod-1", UsagePercentage: 95},
	}, map[string]string{"default/pvc-1": "pv-1"}, true, false)

	// Verify no ConfigMap was created (SampleNode should not persist)
	_, err := client.CoreV1().ConfigMaps("kwatch").Get(context.Background(), "kwatch-pvc", metav1.GetOptions{})
	assert.NotNil(t, err, "no ConfigMap should exist before persist is called")

	// Now call persist - should create the ConfigMap
	m.persist(context.Background())

	cm, err := client.CoreV1().ConfigMaps("kwatch").Get(context.Background(), "kwatch-pvc", metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, cm)

	// Apply another sample and persist again
	m.apply([]*PvcUsage{
		{Name: "pvc-1", PVName: "pv-1", Namespace: "default", PodName: "pod-1", UsagePercentage: 96},
	}, map[string]string{"default/pvc-1": "pv-1"}, true, false)
	m.persist(context.Background())

	// Simulate SampleNode calls (apply only, no persist) — these should not create extra writes
	m.apply([]*PvcUsage{
		{Name: "pvc-1", PVName: "pv-1", Namespace: "default", PodName: "pod-1", UsagePercentage: 97},
	}, map[string]string{"default/pvc-1": "pv-1"}, true, false)
	m.apply([]*PvcUsage{
		{Name: "pvc-1", PVName: "pv-1", Namespace: "default", PodName: "pod-1", UsagePercentage: 98},
	}, map[string]string{"default/pvc-1": "pv-1"}, true, false)

	// Verify ConfigMap has the value from the LAST persist, not the intermediate SampleNode calls
	loaded := sm.GetPvcUsage(context.Background())
	assert.Equal(t, 96.0, loaded["pv-1"].Pct, "last persisted value should be from the persist call, not SampleNode")
}

// ── B1: lastUsage eviction tests ──────────────────────────────

func TestApplySubThresholdDoesNotCache(t *testing.T) {
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80, ClearThreshold: 75}
	corr := newTestCorrelator()
	m := newTestPvcMonitor(cfg, corr)

	// Multiple sub-threshold PVCs: none should appear in lastUsage
	m.apply([]*PvcUsage{
		{Name: "pvc-1", PVName: "pv-1", Namespace: "default", PodName: "pod-1", UsagePercentage: 10},
		{Name: "pvc-2", PVName: "pv-2", Namespace: "default", PodName: "pod-2", UsagePercentage: 50},
		{Name: "pvc-3", PVName: "pv-3", Namespace: "default", PodName: "pod-3", UsagePercentage: 74},
	}, map[string]string{"default/pvc-1": "pv-1", "default/pvc-2": "pv-2", "default/pvc-3": "pv-3"}, false, true)

	assert.Equal(t, 0, len(m.lastUsage), "sub-threshold PVCs must not be cached")
	assert.Equal(t, 0, len(m.notifiedPvc), "sub-threshold PVCs must not be notified")
}

func TestApplyHighDroppingBelowClearWhileMountedEvicts(t *testing.T) {
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80, ClearThreshold: 75}
	corr := newTestCorrelator()
	m := newTestPvcMonitor(cfg, corr)

	// First cycle: high usage — must be cached + notified
	m.apply([]*PvcUsage{
		{Name: "pvc-1", PVName: "pv-1", Namespace: "default", PodName: "pod-1", UsagePercentage: 95},
	}, map[string]string{"default/pvc-1": "pv-1"}, false, true)

	assert.Contains(t, m.lastUsage, "pv-1")
	assert.True(t, m.notifiedPvc["pv-1"])

	// Second cycle: still mounted but below clear (e.g. 50%)
	// Must evict from lastUsage and resolve the incident
	m.apply([]*PvcUsage{
		{Name: "pvc-1", PVName: "pv-1", Namespace: "default", PodName: "pod-1", UsagePercentage: 50},
	}, map[string]string{"default/pvc-1": "pv-1"}, false, true)

	assert.NotContains(t, m.lastUsage, "pv-1", "dropped below clear must evict from lastUsage")
	assert.False(t, m.notifiedPvc["pv-1"], "dropped below clear must resolve")
}

func TestApplyBetweenClearAndThresholdHolds(t *testing.T) {
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80, ClearThreshold: 75}
	corr := newTestCorrelator()
	m := newTestPvcMonitor(cfg, corr)

	// First cycle: high usage (95) — fires
	m.apply([]*PvcUsage{
		{Name: "pvc-1", PVName: "pv-1", Namespace: "default", PodName: "pod-1", UsagePercentage: 95},
	}, map[string]string{"default/pvc-1": "pv-1"}, false, true)

	assert.True(t, m.notifiedPvc["pv-1"])
	assert.Contains(t, m.lastUsage, "pv-1")

	// Second cycle: usage in hold band (clear ≤ usage < threshold, e.g. 78)
	m.apply([]*PvcUsage{
		{Name: "pvc-1", PVName: "pv-1", Namespace: "default", PodName: "pod-1", UsagePercentage: 78},
	}, map[string]string{"default/pvc-1": "pv-1"}, false, true)

	// Must still be in lastUsage (≥ clear) and still notified
	assert.Contains(t, m.lastUsage, "pv-1", "hold-band PVC must remain in lastUsage")
	assert.True(t, m.notifiedPvc["pv-1"], "hold-band PVC must remain firing")
}

// ── B4: cleanup prunes stale lastNodeSample ───────────────────

func TestCleanupPrunesStaleLastNodeSample(t *testing.T) {
	m := newTestPvcMonitor(&config.PvcMonitor{Enabled: true, Threshold: 80}, nil)

	// Seed with an old entry (well past the 10 min cutoff)
	m.mu.Lock()
	m.lastNodeSample = map[string]time.Time{
		"old-node":  time.Now().Add(-30 * time.Minute),
		"fresh-node": time.Now().Add(-1 * time.Minute),
	}
	m.mu.Unlock()

	m.cleanup()

	m.mu.RLock()
	assert.NotContains(t, m.lastNodeSample, "old-node", "stale entry must be pruned")
	assert.Contains(t, m.lastNodeSample, "fresh-node", "recent entry must survive")
	m.mu.RUnlock()
}

func TestCleanupEmptyLastNodeSampleNoPanic(t *testing.T) {
	m := newTestPvcMonitor(&config.PvcMonitor{Enabled: true, Threshold: 80}, nil)

	// lastNodeSample is nil
	m.cleanup()
	// no panic

	m.mu.Lock()
	m.lastNodeSample = make(map[string]time.Time)
	m.mu.Unlock()
	m.cleanup()
	// no panic, nothing to prune
}

// ── B5/B8: isSweep + firstScan + SampleNode signal gating ────

func TestApplySampleNodeDoesNotClearFirstScan(t *testing.T) {
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80}
	corr := newTestCorrelator()
	m := newTestPvcMonitor(cfg, corr)
	m.firstScan = true

	// SampleNode calls apply with isSweep=false
	m.apply([]*PvcUsage{
		{Name: "pvc-1", PVName: "pv-1", Namespace: "default", PodName: "pod-1", UsagePercentage: 95},
	}, map[string]string{"default/pvc-1": "pv-1"}, true, false)

	assert.True(t, m.firstScan, "SampleNode must NOT clear firstScan")
	assert.True(t, m.notifiedPvc["pv-1"], "high PVC must be tracked even in SampleNode")
}

func TestApplySweepClearsFirstScan(t *testing.T) {
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80}
	corr := newTestCorrelator()
	m := newTestPvcMonitor(cfg, corr)
	m.firstScan = true

	// Full sweep with isSweep=true
	m.apply([]*PvcUsage{
		{Name: "pvc-1", PVName: "pv-1", Namespace: "default", PodName: "pod-1", UsagePercentage: 95},
	}, map[string]string{"default/pvc-1": "pv-1"}, false, true)

	assert.False(t, m.firstScan, "full sweep must clear firstScan")
}

func TestApplyFirstScanSuppressesSampleNodeSignal(t *testing.T) {
	corr := newTestCorrelator()
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80}
	m := newTestPvcMonitor(cfg, corr)
	m.firstScan = true

	// SampleNode with firstScan=true: should suppress signal AND keep firstScan=true
	m.apply([]*PvcUsage{
		{Name: "pvc-1", PVName: "pv-1", Namespace: "default", PodName: "pod-1", UsagePercentage: 95},
	}, map[string]string{"default/pvc-1": "pv-1"}, true, false)

	assert.True(t, m.firstScan, "firstScan must survive SampleNode call")

	// No incident in correlator
	snap := corr.Snapshot()
	assert.Equal(t, 0, len(snap), "no incident should exist in correlator during firstScan even from SampleNode")
}

func TestApplySampleNodeOnlySignalsRisingEdge(t *testing.T) {
	corr := newTestCorrelator()
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80}
	m := newTestPvcMonitor(cfg, corr)

	// First SampleNode: rising edge → should create incident
	m.apply([]*PvcUsage{
		{Name: "pvc-1", PVName: "pv-1", Namespace: "default", PodName: "pod-1", UsagePercentage: 95},
	}, map[string]string{"default/pvc-1": "pv-1"}, true, false)

	snap := corr.Snapshot()
	assert.Equal(t, 1, len(snap), "first SampleNode call should create incident")
	assert.Equal(t, 1, snap[0].Count, "Count should be 1 after first signal")

	// Subsequent SampleNode calls on same high PVC: must NOT increment Count
	// (isSweep=false, wasNotified=true → edgeAction dedup)
	for i := 0; i < 5; i++ {
		m.apply([]*PvcUsage{
			{Name: "pvc-1", PVName: "pv-1", Namespace: "default", PodName: "pod-1", UsagePercentage: 96},
		}, map[string]string{"default/pvc-1": "pv-1"}, true, false)
	}

	snap = corr.Snapshot()
	assert.Equal(t, 1, len(snap), "incident should still be the only one")
	assert.Equal(t, 1, snap[0].Count, "Count must NOT inflate from repeated SampleNode calls")
}

func TestApplySweepReSignalsUnconditionally(t *testing.T) {
	corr := newTestCorrelator()
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80}
	m := newTestPvcMonitor(cfg, corr)

	// Full sweep creates incident
	m.apply([]*PvcUsage{
		{Name: "pvc-1", PVName: "pv-1", Namespace: "default", PodName: "pod-1", UsagePercentage: 95},
	}, map[string]string{"default/pvc-1": "pv-1"}, false, true)

	snap := corr.Snapshot()
	assert.Equal(t, 1, len(snap))
	assert.Equal(t, 1, snap[0].Count)

	// Another full sweep: re-signals → edgeAction dedup, but Process still increments Count
	m.apply([]*PvcUsage{
		{Name: "pvc-1", PVName: "pv-1", Namespace: "default", PodName: "pod-1", UsagePercentage: 95},
	}, map[string]string{"default/pvc-1": "pv-1"}, false, true)

	snap = corr.Snapshot()
	assert.Equal(t, 1, len(snap))
	assert.Equal(t, 2, snap[0].Count, "sweep re-signal increments Count (edgeAction dedup is separate)")
}

// ── B9: pvcMap TTL cache ─────────────────────────────────────

func TestPvcMapCachesAndReturnsWithinTTL(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80}
	m := NewPvcMonitor(client, cfg, &alert.AlertManager{}, nil, nil)

	// Create a PVC in the fake cluster
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pvc", Namespace: "default"},
		Spec:       v1.PersistentVolumeClaimSpec{VolumeName: "test-pv"},
	}
	_, err := client.CoreV1().PersistentVolumeClaims("default").Create(ctx, pvc, metav1.CreateOptions{})
	assert.Nil(t, err)

	// First call: should hit API and cache the result
	m1 := m.pvcMap(ctx)
	assert.Equal(t, "test-pv", m1["default/test-pvc"])

	// Delete the PVC from the fake cluster
	err = client.CoreV1().PersistentVolumeClaims("default").Delete(ctx, "test-pvc", metav1.DeleteOptions{})
	assert.Nil(t, err)

	// Second call within TTL: should return cached map (still has the deleted PVC)
	m2 := m.pvcMap(ctx)
	assert.Equal(t, m1, m2, "within TTL must return identical cached map")
	assert.Equal(t, "test-pv", m2["default/test-pvc"], "cached map must still have the deleted PVC")
}

func TestPvcMapRefreshesAfterTTL(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80}
	m := NewPvcMonitor(client, cfg, &alert.AlertManager{}, nil, nil)

	// Create a PVC
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pvc", Namespace: "default"},
		Spec:       v1.PersistentVolumeClaimSpec{VolumeName: "test-pv"},
	}
	_, err := client.CoreV1().PersistentVolumeClaims("default").Create(ctx, pvc, metav1.CreateOptions{})
	assert.Nil(t, err)

	// First call populates cache
	m1 := m.pvcMap(ctx)
	assert.Equal(t, "test-pv", m1["default/test-pvc"])

	// Delete PVC
	err = client.CoreV1().PersistentVolumeClaims("default").Delete(ctx, "test-pvc", metav1.DeleteOptions{})
	assert.Nil(t, err)

	// Force cache expiry by winding time forward past TTL
	m.mu.Lock()
	m.pvByPVCAt = time.Now().Add(-2 * pvByPVCTTL)
	m.mu.Unlock()

	// Third call: should refresh from API — map should now be empty
	m2 := m.pvcMap(ctx)
	assert.NotContains(t, m2, "default/test-pvc", "after TTL must reflect current API state")
}

func TestPvcMapEmptyWithNoPVCs(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80}
	m := NewPvcMonitor(client, cfg, &alert.AlertManager{}, nil, nil)

	result := m.pvcMap(ctx)
	assert.NotNil(t, result)
	assert.Equal(t, 0, len(result), "no PVCs in cluster")
}

func TestPvcMapReturnsMapNotNilOnAPIFailure(t *testing.T) {
	// With a nil client, the API call will fail; pvcMap should fall back to
	// the last good map (nil initially, so returns nil).
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80}
	m := NewPvcMonitor(nil, cfg, &alert.AlertManager{}, nil, nil)

	result := m.pvcMap(context.Background())
	assert.NotNil(t, result, "no client → empty map, not nil")
	assert.Equal(t, 0, len(result), "no client → empty map")
}
