package startup

import (
	"context"
	"testing"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNewStartupManager(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	namespace := "kwatch"
	telemetryCfg := &config.Telemetry{Enabled: false}
	alertCfg := make(map[string]map[string]interface{})
	appCfg := &config.App{}

	sm := NewStartupManager(client, namespace, telemetryCfg, alertCfg, appCfg)
	assert.NotNil(sm)
	assert.NotNil(sm.stateManager)
	assert.NotNil(sm.telemetry)
	assert.NotNil(sm.alertManager)
}

func TestNewStartupManagerWithNilAlertConfig(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	namespace := "kwatch"
	telemetryCfg := &config.Telemetry{Enabled: false}
	appCfg := &config.App{}

	sm := NewStartupManager(client, namespace, telemetryCfg, nil, appCfg)
	assert.NotNil(sm)
	assert.NotNil(sm.alertManager)
}

func TestGetAlertManager(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	namespace := "kwatch"
	telemetryCfg := &config.Telemetry{Enabled: false}
	alertCfg := make(map[string]map[string]interface{})
	appCfg := &config.App{}

	sm := NewStartupManager(client, namespace, telemetryCfg, alertCfg, appCfg)
	assert.NotNil(sm)
	assert.NotNil(sm.GetAlertManager())
}

func TestHandleStartupFirstRun(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	namespace := "kwatch"
	telemetryCfg := &config.Telemetry{Enabled: false}
	alertCfg := make(map[string]map[string]interface{})
	appCfg := &config.App{
		DisableStartupMessage: true,
	}

	sm := NewStartupManager(client, namespace, telemetryCfg, alertCfg, appCfg)
	assert.NotNil(sm)

	err := sm.HandleStartup(context.Background())
	assert.Nil(err)

	isFirstRun, _ := sm.stateManager.IsFirstRun(context.Background())
	assert.False(isFirstRun)

	cm, _ := client.CoreV1().ConfigMaps(namespace).Get(
		context.Background(), "kwatch-state", metav1.GetOptions{})
	assert.NotNil(cm)
	assert.Equal("true", cm.Data["kwatch-init"])
	assert.NotEmpty(cm.Data["cluster-id"])
	assert.NotEmpty(cm.Data["first-run"])
}

func TestHandleStartupUpgrade(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	namespace := "kwatch"

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kwatch-state",
			Namespace: namespace,
		},
		Data: map[string]string{
			"kwatch-init": "true",
			"cluster-id":  "existing-cluster-id",
			"version":     "v0.10.0",
		},
	}
	_, err := client.CoreV1().ConfigMaps(namespace).Create(
		context.Background(), cm, metav1.CreateOptions{})
	assert.Nil(err)

	telemetryCfg := &config.Telemetry{Enabled: false}
	alertCfg := make(map[string]map[string]interface{})
	appCfg := &config.App{
		DisableStartupMessage: true,
	}

	sm := NewStartupManager(client, namespace, telemetryCfg, alertCfg, appCfg)
	assert.NotNil(sm)

	err = sm.HandleStartup(context.Background())
	assert.Nil(err)

	updatedCM, _ := client.CoreV1().ConfigMaps(namespace).Get(
		context.Background(), "kwatch-state", metav1.GetOptions{})
	assert.NotNil(updatedCM)
	assert.Equal("existing-cluster-id", updatedCM.Data["cluster-id"])
	assert.Equal("dev", updatedCM.Data["version"])
}

func TestHandleStartupPreservesClusterID(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	namespace := "kwatch"

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kwatch-state",
			Namespace: namespace,
		},
		Data: map[string]string{
			"kwatch-init": "true",
			"cluster-id":  "original-cluster-id",
			"version":     "dev",
		},
	}
	_, err := client.CoreV1().ConfigMaps(namespace).Create(
		context.Background(), cm, metav1.CreateOptions{})
	assert.Nil(err)

	telemetryCfg := &config.Telemetry{Enabled: false}
	alertCfg := make(map[string]map[string]interface{})
	appCfg := &config.App{
		DisableStartupMessage: true,
	}

	sm := NewStartupManager(client, namespace, telemetryCfg, alertCfg, appCfg)

	err = sm.HandleStartup(context.Background())
	assert.Nil(err)

	updatedCM, _ := client.CoreV1().ConfigMaps(namespace).Get(
		context.Background(), "kwatch-state", metav1.GetOptions{})
	assert.Equal("original-cluster-id", updatedCM.Data["cluster-id"])
}

func TestHandleStartupSameVersion(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	namespace := "kwatch"

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kwatch-state",
			Namespace: namespace,
		},
		Data: map[string]string{
			"kwatch-init":    "true",
			"cluster-id":     "test-cluster-id",
			"version":        "dev",
			"telemetry-sent": "true",
		},
	}
	_, err := client.CoreV1().ConfigMaps(namespace).Create(
		context.Background(), cm, metav1.CreateOptions{})
	assert.Nil(err)

	telemetryCfg := &config.Telemetry{Enabled: true}
	alertCfg := make(map[string]map[string]interface{})
	appCfg := &config.App{
		DisableStartupMessage: true,
	}

	sm := NewStartupManager(client, namespace, telemetryCfg, alertCfg, appCfg)

	err = sm.HandleStartup(context.Background())
	assert.Nil(err)

	updatedCM, _ := client.CoreV1().ConfigMaps(namespace).Get(
		context.Background(), "kwatch-state", metav1.GetOptions{})
	assert.Equal("true", updatedCM.Data["telemetry-sent"])
	assert.Equal("dev", updatedCM.Data["version"])
}
