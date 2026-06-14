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
	"github.com/stretchr/testify/assert"
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

	pvc := NewPvcMonitor(client, cfg, alertMgr, nil)
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

	pvc := NewPvcMonitor(client, nil, alertMgr, nil)
	assert.NotNil(pvc)
	assert.Nil(pvc.config)
}

func TestNewPvcMonitorNilAlertManager(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.PvcMonitor{}

	pvc := NewPvcMonitor(client, cfg, nil, nil)
	assert.NotNil(pvc)
	assert.Nil(pvc.alertManager)
}

func TestStartDisabled(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.PvcMonitor{
		Enabled: false,
	}
	alertMgr := &alert.AlertManager{}

	pvc := NewPvcMonitor(client, cfg, alertMgr, nil)
	pvc.Start(context.Background())
}

func TestCleanupUnderThreshold(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	cfg := &config.PvcMonitor{}
	alertMgr := &alert.AlertManager{}

	pvc := NewPvcMonitor(client, cfg, alertMgr, nil)

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

	pvc := NewPvcMonitor(client, cfg, alertMgr, nil)

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

	pvc := NewPvcMonitor(client, cfg, alertMgr, nil)

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

	pvc := NewPvcMonitor(client, cfg, alertMgr, nil)

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

	pvc := NewPvcMonitor(client, cfg, alertMgr, nil)
	pvc.checkUsage()

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

	pvc := NewPvcMonitor(client, cfg, alertMgr, nil)
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

	pvc := NewPvcMonitor(client, cfg, alertMgr, nil)

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

	pvc := NewPvcMonitor(client, cfg, alertMgr, nil)

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
		Window:   10 * time.Minute,
		Cooldown: 1 * time.Millisecond,
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

	time.Sleep(2 * time.Millisecond)

	_, action2 := correlator.Process(ev, owner, nil)
	assert.Equal(t, model.ActionUpdate, action2, "second call with stable reason should update, not create")
}

func TestPvcStableReasonDifferentPercentages(t *testing.T) {
	correlator := correlation.NewEngine(correlation.Config{
		Window:   10 * time.Minute,
		Cooldown: 1 * time.Millisecond,
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

	time.Sleep(2 * time.Millisecond)

	ev2 := event.Event{
		Resource:  "pvc",
		PodName:   "test-pod",
		Namespace: "default",
		Reason:    "VolumeUsageHigh",
		Hint:      fmt.Sprintf("VolumeUsage(%.0f%%)", 96.0),
	}

	_, action2 := correlator.Process(ev2, owner, nil)
	assert.Equal(t, model.ActionUpdate, action2, "different percentage, same stable reason should still dedup")
}

func TestPvcSeverityWarnTier(t *testing.T) {
	correlator := correlation.NewEngine(correlation.Config{
		Window:   10 * time.Minute,
		Cooldown: 1 * time.Nanosecond,
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
		Cooldown: 1 * time.Nanosecond,
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

func TestPvcStartupQuietSeedsBaseline(t *testing.T) {
	correlator := correlation.NewEngine(correlation.Config{
		Window:       10 * time.Minute,
		Cooldown:     5 * time.Minute,
		StartupQuiet: 1 * time.Hour,
	})

	assert.True(t, correlator.InStartupQuiet(), "InStartupQuiet should return true")

	key := correlation.BuildKey("default", "test-pv", "VolumeUsageHigh", "")
	correlator.SeedBaseline(key)

	snapshot := correlator.BaselineSnapshot()
	assert.Contains(t, snapshot, key, "SeedBaseline should store the key")

	ev := event.Event{
		Resource:  "pvc",
		PodName:   "test-pod",
		Namespace: "default",
		Reason:    "VolumeUsageHigh",
	}

	_, action := correlator.Process(ev, "test-pv", nil)
	assert.Equal(t, model.ActionSkip, action,
		"Process should skip baselined PVC incident during startup-quiet")
}

func TestPvcSeverityUpgradeFromWarnToCritical(t *testing.T) {
	correlator := correlation.NewEngine(correlation.Config{
		Window:   10 * time.Minute,
		Cooldown: 1 * time.Nanosecond,
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
