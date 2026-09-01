package state

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
)

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
	_, err := client.CoreV1().ConfigMaps(
		"kwatch",
	).Create(
		context.Background(),
		cm,
		metav1.CreateOptions{},
	)
	assert.Nil(err)

	err = sm.MarkAsInitialized(
		context.Background(),
		"new-cluster-id",
		"v0.11.0",
	)
	assert.Nil(err)

	updatedCM, err := client.CoreV1().ConfigMaps(
		"kwatch",
	).Get(
		context.Background(),
		stateConfigMapName,
		metav1.GetOptions{},
	)
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
	_, err := client.CoreV1().ConfigMaps(
		"kwatch",
	).Create(
		context.Background(),
		cm,
		metav1.CreateOptions{},
	)
	assert.Nil(err)

	err = sm.MarkAsInitialized(context.Background(), "new-id", "v0.11.0")
	assert.Nil(err)

	updatedCM, _ := client.CoreV1().ConfigMaps(
		"kwatch",
	).Get(
		context.Background(),
		stateConfigMapName,
		metav1.GetOptions{},
	)
	assert.Equal("existing-id", updatedCM.Data[clusterIDKey])
	assert.Equal("2024-01-01T00:00:00Z", updatedCM.Data[firstRunKey])
}

func TestUpdateWithRetrySuccess(t *testing.T) {
	assert := assert.New(t)
	client := fake.NewSimpleClientset()

	mgr := NewRetryConfigMapManager(client, "kwatch", "kwatch-state")
	err := mgr.UpdateWithRetry(
		context.Background(),
		func(cm *corev1.ConfigMap) error {
			cm.Data["test-key"] = "test-value"
			return nil
		},
	)

	assert.Nil(err)

	updatedCM, _ := client.CoreV1().ConfigMaps(
		"kwatch",
	).Get(
		context.Background(),
		"kwatch-state",
		metav1.GetOptions{},
	)
	assert.Equal("test-value", updatedCM.Data["test-key"])
}

func TestUpdateWithRetryCreateIfMissing(t *testing.T) {
	assert := assert.New(t)
	client := fake.NewSimpleClientset()

	mgr := NewRetryConfigMapManager(client, "kwatch", "auto-created-cm")
	err := mgr.UpdateWithRetry(
		context.Background(),
		func(cm *corev1.ConfigMap) error {
			cm.Data["my-key"] = "my-value"
			return nil
		},
	)

	assert.Nil(err)

	cm, err := client.CoreV1().ConfigMaps(
		"kwatch",
	).Get(
		context.Background(),
		"auto-created-cm",
		metav1.GetOptions{},
	)
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
	_, _ = client.CoreV1().ConfigMaps(
		"kwatch",
	).Create(
		context.Background(),
		cm,
		metav1.CreateOptions{},
	)

	mgr := NewRetryConfigMapManager(client, "kwatch", "kwatch-state")
	testErr := errors.New("updater error")
	err := mgr.UpdateWithRetry(
		context.Background(),
		func(cm *corev1.ConfigMap) error {
			return testErr
		},
	)

	assert.Equal(testErr, err)
}

func TestIsConflictError(t *testing.T) {
	assert := assert.New(t)

	// A real k8s conflict: the human-readable message has no "conflict" word.
	conflictErr := apierrors.NewConflict(
		schema.GroupResource{Resource: "configmaps"},
		"kwatch",
		errors.New(
			"Operation cannot be fulfilled on configmaps \"kwatch\": the "+
				"object has been modified; please apply your changes to the "+
				"latest version and try again",
		),
	)
	assert.True(apierrors.IsConflict(conflictErr))
	assert.True(
		isConflictError(conflictErr),
		"real k8s conflict must be detected so retries fire",
	)

	// A StatusError with a different reason is not a conflict.
	notFound := apierrors.NewNotFound(
		schema.GroupResource{Resource: "configmaps"},
		"kwatch",
	)
	assert.False(isConflictError(notFound))

	// Plain error messages containing the word "conflict" are NOT conflicts.
	assert.False(isConflictError(errors.New("conflict in configmap")))
	assert.False(isConflictError(errors.New("Conflict detected")))
	assert.False(isConflictError(errors.New("resource was changed")))

	assert.False(isConflictError(nil))
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
	cm, err := client.CoreV1().ConfigMaps(
		"kwatch",
	).Get(
		context.Background(),
		baselineConfigMapName,
		metav1.GetOptions{},
	)
	assert.Nil(err)
	assert.NotNil(cm.BinaryData[baselineKey])
	assert.Equal("", cm.Data[baselineKey])
}

func TestSaveBaselineOverwrites(t *testing.T) {
	assert := assert.New(t)
	client := fake.NewSimpleClientset()

	sm := NewStateManager(client, "kwatch")

	err := sm.SaveBaseline(
		context.Background(),
		map[string]map[string]int64{"key-1": {"p": 100}},
	)
	assert.Nil(err)
	assert.Equal(
		map[string]map[string]int64{"key-1": {"p": 100}},
		sm.GetBaseline(context.Background()),
	)

	err = sm.SaveBaseline(
		context.Background(),
		map[string]map[string]int64{"key-2": {"q": 200}},
	)
	assert.Nil(err)
	assert.Equal(
		map[string]map[string]int64{"key-2": {"q": 200}},
		sm.GetBaseline(context.Background()),
	)
}

func TestSaveAndGetPvcUsage(t *testing.T) {
	assert := assert.New(t)
	client := fake.NewSimpleClientset()

	sm := NewStateManager(client, "kwatch")

	usage := map[string]PvcSample{
		"pv-1": {
			Pct:       95.5,
			Namespace: "default",
			Name:      "pvc-1",
			Seen:      time.Now(),
		},
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
	cm, err := client.CoreV1().ConfigMaps(
		"kwatch",
	).Get(
		context.Background(),
		pvcConfigMapName,
		metav1.GetOptions{},
	)
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

	// Write baseline in the old location (kwatch-state.data[baseline]) as
	// plaintext
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      stateConfigMapName,
			Namespace: "kwatch",
		},
		Data: map[string]string{
			baselineKey: `{"default:dep-1:CrashLoopBackOff:":{"pod-1":1718064000}}`,
		},
	}
	_, err := client.CoreV1().ConfigMaps(
		"kwatch",
	).Create(
		context.Background(),
		cm,
		metav1.CreateOptions{},
	)
	assert.Nil(err)

	sm := NewStateManager(client, "kwatch")

	loaded := sm.GetBaseline(context.Background())
	assert.NotNil(loaded)
	assert.Equal(
		int64(1718064000),
		loaded["default:dep-1:CrashLoopBackOff:"]["pod-1"],
	)
}
