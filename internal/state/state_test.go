package state

import (
	"context"
	"errors"
	"testing"

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

func TestSetNotifiedVersionNoConfigMap(t *testing.T) {
	assert := assert.New(t)
	client := fake.NewSimpleClientset()
	sm := NewStateManager(client, "kwatch")

	err := sm.SetNotifiedVersion(context.Background(), "v2.0.0")
	assert.NotNil(err)
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

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: "kwatch",
		},
		Data: map[string]string{},
	}
	_, _ = client.CoreV1().ConfigMaps("kwatch").Create(context.Background(), cm, metav1.CreateOptions{})

	mgr := NewRetryConfigMapManager(client, "kwatch")
	err := mgr.UpdateWithRetry(context.Background(), func(cm *corev1.ConfigMap) error {
		cm.Data["test-key"] = "test-value"
		return nil
	})

	assert.Nil(err)

	updatedCM, _ := client.CoreV1().ConfigMaps("kwatch").Get(context.Background(), configMapName, metav1.GetOptions{})
	assert.Equal("test-value", updatedCM.Data["test-key"])
}

func TestUpdateWithRetryGetError(t *testing.T) {
	assert := assert.New(t)
	client := fake.NewSimpleClientset()

	mgr := NewRetryConfigMapManager(client, "kwatch")
	err := mgr.UpdateWithRetry(context.Background(), func(cm *corev1.ConfigMap) error {
		cm.Data["test-key"] = "test-value"
		return nil
	})

	assert.NotNil(err)
}

func TestUpdateWithRetryUpdaterError(t *testing.T) {
	assert := assert.New(t)
	client := fake.NewSimpleClientset()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: "kwatch",
		},
		Data: map[string]string{},
	}
	_, _ = client.CoreV1().ConfigMaps("kwatch").Create(context.Background(), cm, metav1.CreateOptions{})

	mgr := NewRetryConfigMapManager(client, "kwatch")
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

	mgr := NewRetryConfigMapManager(client, "test-namespace")
	assert.NotNil(mgr)
	assert.Equal(client, mgr.client)
	assert.Equal("test-namespace", mgr.namespace)
}
