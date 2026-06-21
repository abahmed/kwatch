package state

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNewStateManager(t *testing.T) {
	assert := assert.New(t)
	client := fake.NewSimpleClientset()
	namespace := "kwatch"

	sm := NewStateManager(client, namespace)
	assert.NotNil(sm)
	assert.Equal(namespace, sm.namespace)
	assert.NotNil(sm.stateMgr)
	assert.NotNil(sm.baselineMgr)
	assert.NotNil(sm.pvcMgr)
}

func TestIsFirstRunNoConfigMap(t *testing.T) {
	assert := assert.New(t)
	client := fake.NewSimpleClientset()
	sm := NewStateManager(client, "kwatch")

	isFirstRun, err := sm.IsFirstRun(context.Background())
	assert.Nil(err)
	assert.True(isFirstRun)
}

func TestIsFirstRunWithConfigMap(t *testing.T) {
	assert := assert.New(t)
	client := fake.NewSimpleClientset()
	sm := NewStateManager(client, "kwatch")

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      stateConfigMapName,
			Namespace: "kwatch",
		},
		Data: map[string]string{
			initKey: "true",
		},
	}
	_, err := client.CoreV1().ConfigMaps("kwatch").Create(context.Background(), cm, metav1.CreateOptions{})
	assert.Nil(err)

	isFirstRun, err := sm.IsFirstRun(context.Background())
	assert.Nil(err)
	assert.False(isFirstRun)
}

func TestGetStoredVersionNoConfigMap(t *testing.T) {
	assert := assert.New(t)
	client := fake.NewSimpleClientset()
	sm := NewStateManager(client, "kwatch")

	version := sm.GetStoredVersion(context.Background())
	assert.Equal("", version)
}

func TestGetStoredVersionWithConfigMap(t *testing.T) {
	assert := assert.New(t)
	client := fake.NewSimpleClientset()
	sm := NewStateManager(client, "kwatch")

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      stateConfigMapName,
			Namespace: "kwatch",
		},
		Data: map[string]string{
			versionKey: "v0.10.0",
		},
	}
	_, err := client.CoreV1().ConfigMaps("kwatch").Create(context.Background(), cm, metav1.CreateOptions{})
	assert.Nil(err)

	version := sm.GetStoredVersion(context.Background())
	assert.Equal("v0.10.0", version)
}

func TestEnsureClusterIDNoConfigMap(t *testing.T) {
	assert := assert.New(t)
	client := fake.NewSimpleClientset()
	sm := NewStateManager(client, "kwatch")

	clusterID, err := sm.EnsureClusterID(context.Background())
	assert.Nil(err)
	assert.NotEmpty(clusterID)
	assert.Len(clusterID, 36)
}

func TestEnsureClusterIDPreservesExisting(t *testing.T) {
	assert := assert.New(t)
	client := fake.NewSimpleClientset()
	sm := NewStateManager(client, "kwatch")

	existingID := "existing-cluster-id"
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      stateConfigMapName,
			Namespace: "kwatch",
		},
		Data: map[string]string{
			clusterIDKey: existingID,
		},
	}
	_, err := client.CoreV1().ConfigMaps("kwatch").Create(context.Background(), cm, metav1.CreateOptions{})
	assert.Nil(err)

	clusterID, err := sm.EnsureClusterID(context.Background())
	assert.Nil(err)
	assert.Equal(existingID, clusterID)
}

func TestMarkAsInitializedCreateConfigMap(t *testing.T) {
	assert := assert.New(t)
	client := fake.NewSimpleClientset()
	sm := NewStateManager(client, "kwatch")

	err := sm.MarkAsInitialized(context.Background(), "test-cluster-id", "v0.11.0")
	assert.Nil(err)

	cm, err := client.CoreV1().ConfigMaps("kwatch").Get(context.Background(), stateConfigMapName, metav1.GetOptions{})
	assert.Nil(err)
	assert.NotNil(cm)
	assert.Equal("true", cm.Data[initKey])
	assert.Equal("test-cluster-id", cm.Data[clusterIDKey])
	assert.Equal("v0.11.0", cm.Data[versionKey])
	assert.NotEmpty(cm.Data[firstRunKey])
}

func TestMarkAsInitializedUpdateConfigMap(t *testing.T) {
	assert := assert.New(t)
	client := fake.NewSimpleClientset()
	sm := NewStateManager(client, "kwatch")

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      stateConfigMapName,
			Namespace: "kwatch",
		},
		Data: map[string]string{
			initKey:      "true",
			clusterIDKey: "old-cluster-id",
			versionKey:   "v0.10.0",
		},
	}
	_, err := client.CoreV1().ConfigMaps("kwatch").Create(context.Background(), cm, metav1.CreateOptions{})
	assert.Nil(err)

	err = sm.MarkAsInitialized(context.Background(), "new-cluster-id", "v0.11.0")
	assert.Nil(err)

	updatedCM, err := client.CoreV1().ConfigMaps("kwatch").Get(context.Background(), stateConfigMapName, metav1.GetOptions{})
	assert.Nil(err)
	assert.Equal("true", updatedCM.Data[initKey])
	assert.Equal("old-cluster-id", updatedCM.Data[clusterIDKey])
	assert.Equal("v0.11.0", updatedCM.Data[versionKey])
}

func TestGetClusterIDNoConfigMap(t *testing.T) {
	assert := assert.New(t)
	client := fake.NewSimpleClientset()
	sm := NewStateManager(client, "kwatch")

	clusterID, err := sm.GetClusterID(context.Background())
	assert.NotNil(err)
	assert.Empty(clusterID)
}

func TestGetClusterIDWithConfigMap(t *testing.T) {
	assert := assert.New(t)
	client := fake.NewSimpleClientset()
	sm := NewStateManager(client, "kwatch")

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      stateConfigMapName,
			Namespace: "kwatch",
		},
		Data: map[string]string{
			clusterIDKey: "test-id-123",
		},
	}
	_, err := client.CoreV1().ConfigMaps("kwatch").Create(context.Background(), cm, metav1.CreateOptions{})
	assert.Nil(err)

	clusterID, err := sm.GetClusterID(context.Background())
	assert.Nil(err)
	assert.Equal("test-id-123", clusterID)
}

func TestGetNotifiedVersionNoConfigMap(t *testing.T) {
	assert := assert.New(t)
	client := fake.NewSimpleClientset()
	sm := NewStateManager(client, "kwatch")

	version := sm.GetNotifiedVersion(context.Background())
	assert.Equal("", version)
}

func TestGetNotifiedVersionWithConfigMap(t *testing.T) {
	assert := assert.New(t)
	client := fake.NewSimpleClientset()
	sm := NewStateManager(client, "kwatch")

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      stateConfigMapName,
			Namespace: "kwatch",
		},
		Data: map[string]string{
			notifiedVersionKey: "v2.0.0",
		},
	}
	_, err := client.CoreV1().ConfigMaps("kwatch").Create(context.Background(), cm, metav1.CreateOptions{})
	assert.Nil(err)

	version := sm.GetNotifiedVersion(context.Background())
	assert.Equal("v2.0.0", version)
}

func TestSetNotifiedVersionCreatesConfigMap(t *testing.T) {
	assert := assert.New(t)
	client := fake.NewSimpleClientset()
	sm := NewStateManager(client, "kwatch")

	err := sm.SetNotifiedVersion(context.Background(), "v2.0.0")
	assert.Nil(err)

	version := sm.GetNotifiedVersion(context.Background())
	assert.Equal("v2.0.0", version)
}

func TestSetNotifiedVersionSuccess(t *testing.T) {
	assert := assert.New(t)
	client := fake.NewSimpleClientset()
	sm := NewStateManager(client, "kwatch")

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      stateConfigMapName,
			Namespace: "kwatch",
		},
		Data: map[string]string{},
	}
	_, err := client.CoreV1().ConfigMaps("kwatch").Create(context.Background(), cm, metav1.CreateOptions{})
	assert.Nil(err)

	err = sm.SetNotifiedVersion(context.Background(), "v2.0.0")
	assert.Nil(err)

	version := sm.GetNotifiedVersion(context.Background())
	assert.Equal("v2.0.0", version)
}

func TestSetNotifiedVersionUpdatesExisting(t *testing.T) {
	assert := assert.New(t)
	client := fake.NewSimpleClientset()
	sm := NewStateManager(client, "kwatch")

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      stateConfigMapName,
			Namespace: "kwatch",
		},
		Data: map[string]string{
			notifiedVersionKey: "v1.0.0",
		},
	}
	_, err := client.CoreV1().ConfigMaps("kwatch").Create(context.Background(), cm, metav1.CreateOptions{})
	assert.Nil(err)

	err = sm.SetNotifiedVersion(context.Background(), "v2.0.0")
	assert.Nil(err)

	version := sm.GetNotifiedVersion(context.Background())
	assert.Equal("v2.0.0", version)
}

func TestMarkAsInitializedUpdateMissingKeys(t *testing.T) {
	assert := assert.New(t)
	client := fake.NewSimpleClientset()
	sm := NewStateManager(client, "kwatch")

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      stateConfigMapName,
			Namespace: "kwatch",
		},
		Data: map[string]string{
			versionKey: "v0.10.0",
		},
	}
	_, err := client.CoreV1().ConfigMaps("kwatch").Create(context.Background(), cm, metav1.CreateOptions{})
	assert.Nil(err)

	err = sm.MarkAsInitialized(context.Background(), "new-cluster-id", "v0.11.0")
	assert.Nil(err)

	updatedCM, err := client.CoreV1().ConfigMaps("kwatch").Get(context.Background(), stateConfigMapName, metav1.GetOptions{})
	assert.Nil(err)
	assert.Equal("true", updatedCM.Data[initKey])
	assert.Equal("new-cluster-id", updatedCM.Data[clusterIDKey])
	assert.NotEmpty(updatedCM.Data[firstRunKey])
	assert.Equal("v0.11.0", updatedCM.Data[versionKey])
}

func TestMarkAsInitializedPreservesExistingClusterID(t *testing.T) {
	assert := assert.New(t)
	client := fake.NewSimpleClientset()
	sm := NewStateManager(client, "kwatch")

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      stateConfigMapName,
			Namespace: "kwatch",
		},
		Data: map[string]string{
			initKey:      "true",
			clusterIDKey: "existing-id",
			firstRunKey:  "2024-01-01T00:00:00Z",
		},
	}
	_, err := client.CoreV1().ConfigMaps("kwatch").Create(context.Background(), cm, metav1.CreateOptions{})
	assert.Nil(err)

	err = sm.MarkAsInitialized(context.Background(), "new-id", "v0.11.0")
	assert.Nil(err)

	updatedCM, _ := client.CoreV1().ConfigMaps("kwatch").Get(context.Background(), stateConfigMapName, metav1.GetOptions{})
	assert.Equal("existing-id", updatedCM.Data[clusterIDKey])
	assert.Equal("2024-01-01T00:00:00Z", updatedCM.Data[firstRunKey])
}

func TestUpdateWithRetrySuccess(t *testing.T) {
	assert := assert.New(t)
	client := fake.NewSimpleClientset()

	mgr := NewRetryConfigMapManager(client, "kwatch", "kwatch-state")
	err := mgr.UpdateWithRetry(context.Background(), func(cm *corev1.ConfigMap) error {
		cm.Data["test-key"] = "test-value"
		return nil
	})

	assert.Nil(err)

	updatedCM, _ := client.CoreV1().ConfigMaps("kwatch").Get(context.Background(), "kwatch-state", metav1.GetOptions{})
	assert.Equal("test-value", updatedCM.Data["test-key"])
}

func TestUpdateWithRetryCreateIfMissing(t *testing.T) {
	assert := assert.New(t)
	client := fake.NewSimpleClientset()

	mgr := NewRetryConfigMapManager(client, "kwatch", "auto-created-cm")
	err := mgr.UpdateWithRetry(context.Background(), func(cm *corev1.ConfigMap) error {
		cm.Data["my-key"] = "my-value"
		return nil
	})

	assert.Nil(err)

	cm, err := client.CoreV1().ConfigMaps("kwatch").Get(context.Background(), "auto-created-cm", metav1.GetOptions{})
	assert.Nil(err)
	assert.Equal("my-value", cm.Data["my-key"])
}

func TestUpdateWithRetryUpdaterError(t *testing.T) {
	assert := assert.New(t)
	client := fake.NewSimpleClientset()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kwatch-state",
			Namespace: "kwatch",
		},
		Data: map[string]string{},
	}
	_, _ = client.CoreV1().ConfigMaps("kwatch").Create(context.Background(), cm, metav1.CreateOptions{})

	mgr := NewRetryConfigMapManager(client, "kwatch", "kwatch-state")
	testErr := errors.New("updater error")
	err := mgr.UpdateWithRetry(context.Background(), func(cm *corev1.ConfigMap) error {
		return testErr
	})

	assert.Equal(testErr, err)
}

func TestIsConflictError(t *testing.T) {
	assert := assert.New(t)

	conflictErr1 := errors.New("conflict in configmap")
	assert.True(isConflictError(conflictErr1))

	conflictErr2 := errors.New("Conflict detected")
	assert.True(isConflictError(conflictErr2))

	conflictErr3 := errors.New("resource was changed")
	assert.True(isConflictError(conflictErr3))

	normalErr := errors.New("not found")
	assert.False(isConflictError(normalErr))

	assert.False(isConflictError(nil))

	conflictErr := &ConflictError{Message: "conflict detected"}
	assert.True(isConflictError(conflictErr))
}

func TestConflictError(t *testing.T) {
	assert := assert.New(t)

	err := &ConflictError{Message: "test conflict error"}
	assert.Equal("test conflict error", err.Error())
}

func TestNewRetryConfigMapManager(t *testing.T) {
	assert := assert.New(t)
	client := fake.NewSimpleClientset()

	mgr := NewRetryConfigMapManager(client, "test-namespace", "test-cm")
	assert.NotNil(mgr)
	assert.Equal(client, mgr.client)
	assert.Equal("test-namespace", mgr.namespace)
	assert.Equal("test-cm", mgr.configName)
}

func TestGetBaselineNoConfigMap(t *testing.T) {
	assert := assert.New(t)
	client := fake.NewSimpleClientset()
	sm := NewStateManager(client, "kwatch")

	result := sm.GetBaseline(context.Background())
	assert.Nil(result)
}

func TestSaveAndGetBaseline(t *testing.T) {
	assert := assert.New(t)
	client := fake.NewSimpleClientset()

	sm := NewStateManager(client, "kwatch")

	baseline := map[string]map[string]int64{
		"default:deploy-1:CrashLoopBackOff:app": {"pod-1": 1718064000},
		"default:sts-0:OOMKilled:web":           {"pod-2": 1718065000},
	}
	err := sm.SaveBaseline(context.Background(), baseline)
	assert.Nil(err)

	loaded := sm.GetBaseline(context.Background())
	assert.NotNil(loaded)
	assert.Equal(baseline, loaded)

	// Verify it's stored in kwatch-baseline, NOT kwatch-state
	cm, err := client.CoreV1().ConfigMaps("kwatch").Get(context.Background(), baselineConfigMapName, metav1.GetOptions{})
	assert.Nil(err)
	assert.NotNil(cm.BinaryData[baselineKey])
	assert.Equal("", cm.Data[baselineKey])
}

func TestSaveBaselineOverwrites(t *testing.T) {
	assert := assert.New(t)
	client := fake.NewSimpleClientset()

	sm := NewStateManager(client, "kwatch")

	err := sm.SaveBaseline(context.Background(), map[string]map[string]int64{"key-1": {"p": 100}})
	assert.Nil(err)
	assert.Equal(map[string]map[string]int64{"key-1": {"p": 100}}, sm.GetBaseline(context.Background()))

	err = sm.SaveBaseline(context.Background(), map[string]map[string]int64{"key-2": {"q": 200}})
	assert.Nil(err)
	assert.Equal(map[string]map[string]int64{"key-2": {"q": 200}}, sm.GetBaseline(context.Background()))
}

func TestSaveAndGetPvcUsage(t *testing.T) {
	assert := assert.New(t)
	client := fake.NewSimpleClientset()

	sm := NewStateManager(client, "kwatch")

	usage := map[string]PvcSample{
		"pv-1": {Pct: 95.5, Namespace: "default", Name: "pvc-1", Seen: time.Now()},
		"pv-2": {Pct: 82.0, Namespace: "prod", Name: "pvc-2", Seen: time.Now()},
	}
	err := sm.SavePvcUsage(context.Background(), usage)
	assert.Nil(err)

	loaded := sm.GetPvcUsage(context.Background())
	assert.NotNil(loaded)
	assert.Equal(usage["pv-1"].Pct, loaded["pv-1"].Pct)
	assert.Equal(usage["pv-1"].Namespace, loaded["pv-1"].Namespace)
	assert.Equal(usage["pv-1"].Name, loaded["pv-1"].Name)
	assert.Equal(usage["pv-2"].Pct, loaded["pv-2"].Pct)

	// Verify it's stored in kwatch-pvc, not kwatch-state
	cm, err := client.CoreV1().ConfigMaps("kwatch").Get(context.Background(), pvcConfigMapName, metav1.GetOptions{})
	assert.Nil(err)
	assert.NotNil(cm.BinaryData[pvcUsageKey])
}

func TestGetPvcUsageNoConfigMap(t *testing.T) {
	assert := assert.New(t)
	client := fake.NewSimpleClientset()
	sm := NewStateManager(client, "kwatch")

	result := sm.GetPvcUsage(context.Background())
	assert.Nil(result)
}

func TestLegacyBaselineMigration(t *testing.T) {
	assert := assert.New(t)
	client := fake.NewSimpleClientset()

	// Write baseline in the old location (kwatch-state.data[baseline]) as plaintext
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      stateConfigMapName,
			Namespace: "kwatch",
		},
		Data: map[string]string{
			baselineKey: `{"default:dep-1:CrashLoopBackOff:":{"pod-1":1718064000}}`,
		},
	}
	_, err := client.CoreV1().ConfigMaps("kwatch").Create(context.Background(), cm, metav1.CreateOptions{})
	assert.Nil(err)

	sm := NewStateManager(client, "kwatch")

	loaded := sm.GetBaseline(context.Background())
	assert.NotNil(loaded)
	assert.Equal(int64(1718064000), loaded["default:dep-1:CrashLoopBackOff:"]["pod-1"])
}

func TestEngineBackedBaselineRoundTrip(t *testing.T) {
	assert := assert.New(t)
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	sm := NewStateManager(client, "kwatch")

	// Save a realistic baseline (same format as controller.buildSeenSet produces)
	baseline := map[string]map[string]int64{
		"default:dep-1:CrashLoopBackOff:": {"pod-1": time.Now().Unix()},
	}
	err := sm.SaveBaseline(ctx, baseline)
	assert.Nil(err)

	// Load baseline — simulates startup reload from ConfigMap
	loaded := sm.GetBaseline(ctx)
	assert.NotNil(loaded)
	assert.Equal(baseline, loaded)

	// Feed loaded baseline into correlation engine (as main.go does)
	e := correlation.NewEngine(correlation.Config{
		Window:   10 * time.Minute,
		Baseline: loaded,
	})

	// Previously seen pod+container should be suppressed
	ev1 := event.Event{
		PodName: "pod-1", Namespace: "default",
		Reason: "CrashLoopBackOff", ContainerName: "app",
	}
	_, action := e.Process(ev1, "dep-1", &model.ContainerState{RestartCount: 1})
	assert.Equal(model.ActionSkip, action, "baselined pod must be suppressed")

	// A new pod for the same owner+reason should create an incident
	ev2 := event.Event{
		PodName: "pod-2", Namespace: "default",
		Reason: "CrashLoopBackOff", ContainerName: "app",
	}
	_, action = e.Process(ev2, "dep-1", &model.ContainerState{RestartCount: 1})
	assert.Equal(model.ActionCreate, action, "unseen pod must create incident")
}

func TestSaveBaselineTooLarge(t *testing.T) {
	assert := assert.New(t)
	client := fake.NewSimpleClientset()
	sm := NewStateManager(client, "kwatch")

	// Build a baseline large enough that even gzipped it exceeds
	// baselineMaxBytes (~1,032,192). Use many entries with unique keys
	// to minimise gzip leverage.
	large := make(map[string]map[string]int64)
	// Each entry: "k-<N>": {"p-<N>": <ts>} ≈ 35 uncompressed bytes.
	// 100,000 entries ~ 3.5 MB uncompressed; gzip of low-entropy data
	// still compresses ~5× → ~700 KB (may or may not exceed 1 MB).
	// Use 200,000 entries with varying key content to reduce compressibility.
	for i := 0; i < 200000; i++ {
		key := fmt.Sprintf("k-%08d", i)
		large[key] = map[string]int64{fmt.Sprintf("p-%08d", i): int64(1718064000 + i)}
	}

	err := sm.SaveBaseline(context.Background(), large)
	assert.NotNil(err, "oversized baseline should be rejected")
	assert.Contains(err.Error(), "exceeds budget")
}
