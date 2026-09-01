package pvc

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
)

func TestApplySweepClearsFirstScan(t *testing.T) {
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80}
	corr := newTestCorrelator()
	m := newTestPvcMonitor(cfg, corr)
	m.firstScan = true

	// Full sweep with isSweep=true
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

	assert.False(
		t,
		m.firstScan,
		"full sweep must clear firstScan",
	)
}

func TestApplySweepReSignalsUnconditionally(t *testing.T) {
	corr := newTestCorrelator()
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80}
	m := newTestPvcMonitor(cfg, corr)

	// Full sweep creates incident
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

	snap := corr.Snapshot()
	assert.Equal(t, 1, len(snap))
	assert.Equal(t, 1, snap[0].Count)

	// A full sweep re-signals without duplicate edge actions.
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

	snap = corr.Snapshot()
	assert.Equal(t, 1, len(snap))
	assert.Equal(
		t,
		2, snap[0].Count,
		"sweep re-signal increments Count (edgeAction dedup is separate)",
	)
}

// Focused PVC monitor tests.

func TestPvcMapCachesAndReturnsWithinTTL(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80}
	m := NewPvcMonitor(client, cfg, nil, nil)

	// Create a PVC in the fake cluster
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pvc", Namespace: "default"},
		Spec:       v1.PersistentVolumeClaimSpec{VolumeName: "test-pv"},
	}
	_, err := client.CoreV1().PersistentVolumeClaims("default").Create(
		ctx, pvc, metav1.CreateOptions{},
	)
	assert.Nil(t, err)

	// First call: should hit API and cache the result
	m1 := m.pvcMap(ctx)
	assert.Equal(t, "test-pv", m1["default/test-pvc"])

	// Delete the PVC from the fake cluster
	err = client.CoreV1().PersistentVolumeClaims("default").Delete(
		ctx, "test-pvc", metav1.DeleteOptions{},
	)
	assert.Nil(t, err)

	// Second call within TTL: should return cached map (still has the deleted PVC)
	m2 := m.pvcMap(ctx)
	assert.Equal(
		t,
		m1, m2,
		"within TTL must return identical cached map",
	)
	assert.Equal(
		t,
		"test-pv", m2["default/test-pvc"],
		"cached map must still have the deleted PVC",
	)
}

func TestPvcMapRefreshesAfterTTL(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80}
	m := NewPvcMonitor(client, cfg, nil, nil)

	// Create a PVC
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pvc", Namespace: "default"},
		Spec:       v1.PersistentVolumeClaimSpec{VolumeName: "test-pv"},
	}
	_, err := client.CoreV1().PersistentVolumeClaims("default").Create(
		ctx, pvc, metav1.CreateOptions{},
	)
	assert.Nil(t, err)

	// First call populates cache
	m1 := m.pvcMap(ctx)
	assert.Equal(t, "test-pv", m1["default/test-pvc"])

	// Delete PVC
	err = client.CoreV1().PersistentVolumeClaims("default").Delete(
		ctx, "test-pvc", metav1.DeleteOptions{},
	)
	assert.Nil(t, err)

	// Force cache expiry by winding time forward past TTL
	m.mu.Lock()
	m.pvByPVCAt = time.Now().Add(-2 * pvByPVCTTL)
	m.mu.Unlock()

	// Third call: should refresh from API — map should now be empty
	m2 := m.pvcMap(ctx)
	assert.NotContains(
		t,
		m2, "default/test-pvc",
		"after TTL must reflect current API state",
	)
}

func TestPvcMapEmptyWithNoPVCs(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80}
	m := NewPvcMonitor(client, cfg, nil, nil)

	result := m.pvcMap(ctx)
	assert.NotNil(t, result)
	assert.Equal(
		t,
		0, len(result),
		"no PVCs in cluster",
	)
}

func TestPvcMapReturnsMapNotNilOnAPIFailure(t *testing.T) {
	// With a nil client, the API call will fail; pvcMap should fall back to
	// the last good map (nil initially, so returns nil).
	cfg := &config.PvcMonitor{Enabled: true, Threshold: 80}
	m := NewPvcMonitor(nil, cfg, nil, nil)

	result := m.pvcMap(context.Background())
	assert.NotNil(t, result, "no client → empty map, not nil")
	assert.Equal(
		t,
		0, len(result),
		"no client → empty map",
	)
}
