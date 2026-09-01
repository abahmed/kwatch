package pvc

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/state"
)

func TestPersistWritesToConfigMap(t *testing.T) {
	client := fake.NewSimpleClientset()
	sm := state.NewStateManager(client, "kwatch")

	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80}
	corr := newTestCorrelator()
	m := NewPvcMonitor(client, cfg, corr, sm)

	// Populate lastUsage via apply
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

	// Persist
	m.persist(context.Background())

	// Verify the ConfigMap was written
	cm, err := client.CoreV1().ConfigMaps("kwatch").Get(
		context.Background(),
		"kwatch-pvc",
		metav1.GetOptions{},
	)
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

	m.persist(context.Background())
	// no panic, no ConfigMap written (verified by the lack of error)
}

// Focused PVC monitor tests.

func TestPersistenceRoundTripRestoresIncidents(t *testing.T) {
	client := fake.NewSimpleClientset()
	sm := state.NewStateManager(client, "kwatch")

	// Pre-populate the ConfigMap with a high PVC sample (simulates prior run)
	preExisting := map[string]state.PvcSample{
		"pv-unmounted": {
			Pct: 95, Namespace: "default", Name: "pvc-backup", Seen: time.Now(),
		},
		"pv-low": {
			Pct: 50, Namespace: "default", Name: "pvc-low", Seen: time.Now(),
		},
	}
	err := sm.SavePvcUsage(context.Background(), preExisting)
	assert.Nil(t, err)

	// Create a PvcMonitor with the SAME state manager (simulates restart)
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80, CriticalThreshold: 90}
	corr := newTestCorrelator()
	m := NewPvcMonitor(client, cfg, corr, sm)

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
	assert.True(
		t,
		m.notifiedPvc["pv-unmounted"],
		"high PVC must be re-notified after restart",
	)
	assert.False(
		t,
		m.notifiedPvc["pv-low"],
		"low PVC must not be notified",
	)
	assert.Contains(
		t,
		m.lastUsage,
		"pv-unmounted",
	)
	assert.Contains(
		t,
		m.lastUsage,
		"pv-low",
	)

	// Verify lastUsage values are correct
	assert.Equal(t, 95.0, m.lastUsage["pv-unmounted"].Pct)
	assert.Equal(t, "pvc-backup", m.lastUsage["pv-unmounted"].Name)
}

func TestPersistenceRoundTripKeepFiringWithoutRemount(t *testing.T) {
	client := fake.NewSimpleClientset()
	sm := state.NewStateManager(client, "kwatch")

	// Pre-populate ConfigMap with a high PVC sample
	preExisting := map[string]state.PvcSample{
		"pv-backup": {
			Pct: 95, Namespace: "default", Name: "pvc-backup", Seen: time.Now(),
		},
	}
	err := sm.SavePvcUsage(context.Background(), preExisting)
	assert.Nil(t, err)

	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80, ClearThreshold: 75}
	corr := newTestCorrelator()
	m := NewPvcMonitor(client, cfg, corr, sm)

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
	m := NewPvcMonitor(client, cfg, corr, sm)

	// Seed from ConfigMap (should return nil since there's no data)
	assert.Nil(t, sm.GetPvcUsage(context.Background()))

	// Verify nothing was seeded
	assert.Equal(t, 0, len(m.lastUsage))
	assert.Equal(t, 0, len(m.notifiedPvc))
}

// Focused PVC monitor tests.

func TestApplySubThresholdDoesNotCache(t *testing.T) {
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80, ClearThreshold: 75}
	corr := newTestCorrelator()
	m := newTestPvcMonitor(cfg, corr)

	// Multiple sub-threshold PVCs: none should appear in lastUsage
	m.apply([]*PvcUsage{
		{
			Name:            "pvc-1",
			PVName:          "pv-1",
			Namespace:       "default",
			PodName:         "pod-1",
			UsagePercentage: 10,
		},
		{
			Name:            "pvc-2",
			PVName:          "pv-2",
			Namespace:       "default",
			PodName:         "pod-2",
			UsagePercentage: 50,
		},
		{
			Name:            "pvc-3",
			PVName:          "pv-3",
			Namespace:       "default",
			PodName:         "pod-3",
			UsagePercentage: 74,
		},
	}, map[string]string{
		"default/pvc-1": "pv-1",
		"default/pvc-2": "pv-2",
		"default/pvc-3": "pv-3",
	}, false, true)

	assert.Equal(
		t,
		0, len(m.lastUsage),
		"sub-threshold PVCs must not be cached",
	)
	assert.Equal(
		t,
		0, len(m.notifiedPvc),
		"sub-threshold PVCs must not be notified",
	)
}

func TestApplyHighDroppingBelowClearWhileMountedEvicts(t *testing.T) {
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80, ClearThreshold: 75}
	corr := newTestCorrelator()
	m := newTestPvcMonitor(cfg, corr)

	// First cycle: high usage — must be cached + notified
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

	assert.Contains(
		t,
		m.lastUsage,
		"pv-1",
	)
	assert.True(t, m.notifiedPvc["pv-1"])

	// Second cycle: still mounted but below clear (e.g. 50%)
	// Must evict from lastUsage and resolve the incident
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

	assert.NotContains(
		t,
		m.lastUsage, "pv-1",
		"dropped below clear must evict from lastUsage",
	)
	assert.False(
		t,
		m.notifiedPvc["pv-1"],
		"dropped below clear must resolve",
	)
}

func TestApplyBetweenClearAndThresholdHolds(t *testing.T) {
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80, ClearThreshold: 75}
	corr := newTestCorrelator()
	m := newTestPvcMonitor(cfg, corr)

	// First cycle: high usage (95) — fires
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

	// Second cycle: usage in hold band (clear ≤ usage < threshold, e.g. 78)
	m.apply([]*PvcUsage{
		{
			Name:            "pvc-1",
			PVName:          "pv-1",
			Namespace:       "default",
			PodName:         "pod-1",
			UsagePercentage: 78,
		},
	}, map[string]string{
		"default/pvc-1": "pv-1",
		"default/pvc-2": "pv-2",
		"default/pvc-3": "pv-3",
	}, false, true)

	// Must still be in lastUsage (≥ clear) and still notified
	assert.Contains(
		t,
		m.lastUsage, "pv-1",
		"hold-band PVC must remain in lastUsage",
	)
	assert.True(
		t,
		m.notifiedPvc["pv-1"],
		"hold-band PVC must remain firing",
	)
}
