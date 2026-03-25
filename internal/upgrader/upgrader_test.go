package upgrader

import (
	"context"
	"testing"

	"github.com/abahmed/kwatch/internal/alert"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/state"
	"github.com/abahmed/kwatch/internal/version"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNewUpgrader(t *testing.T) {
	assert := assert.New(t)

	upgraderConfig := &config.Upgrader{}
	alertMgr := &alertmanager.AlertManager{}
	stateMgr := state.NewStateManager(fake.NewSimpleClientset(), "kwatch")

	u := NewUpgrader(upgraderConfig, alertMgr, stateMgr)
	assert.NotNil(u)
	assert.Equal(upgraderConfig, u.config)
	assert.Equal(alertMgr, u.alertManager)
	assert.Equal(stateMgr, u.stateManager)
}

func TestNewUpgraderNilStateManager(t *testing.T) {
	assert := assert.New(t)

	upgraderConfig := &config.Upgrader{}
	alertMgr := &alertmanager.AlertManager{}

	u := NewUpgrader(upgraderConfig, alertMgr, nil)
	assert.NotNil(u)
	assert.Nil(u.stateManager)
}

func TestCheckUpdatesDisabled(t *testing.T) {
	assert := assert.New(t)

	upgraderConfig := &config.Upgrader{
		DisableUpdateCheck: true,
	}
	alertMgr := &alertmanager.AlertManager{}
	stateMgr := state.NewStateManager(fake.NewSimpleClientset(), "kwatch")

	u := NewUpgrader(upgraderConfig, alertMgr, stateMgr)
	assert.NotNil(u)
}

func TestUpgraderFields(t *testing.T) {
	assert := assert.New(t)

	upgraderConfig := &config.Upgrader{
		DisableUpdateCheck: true,
	}
	alertMgr := &alertmanager.AlertManager{}
	stateMgr := state.NewStateManager(fake.NewSimpleClientset(), "kwatch")

	u := NewUpgrader(upgraderConfig, alertMgr, stateMgr)
	assert.NotNil(u)
	assert.Equal(upgraderConfig, u.config)
	assert.Equal(alertMgr, u.alertManager)
	assert.Equal(stateMgr, u.stateManager)
	assert.True(u.config.DisableUpdateCheck)
}

func TestVersionComparison(t *testing.T) {
	assert := assert.New(t)

	currentVersion := version.Short()
	assert.NotEmpty(currentVersion)
}

func TestUpgraderWithDisabledConfig(t *testing.T) {
	assert := assert.New(t)

	upgraderConfig := &config.Upgrader{
		DisableUpdateCheck: true,
	}
	alertMgr := &alertmanager.AlertManager{}
	stateMgr := state.NewStateManager(fake.NewSimpleClientset(), "kwatch")

	u := NewUpgrader(upgraderConfig, alertMgr, stateMgr)
	assert.NotNil(u)
	assert.True(u.config.DisableUpdateCheck)
}

func TestUpgraderWithEnabledConfig(t *testing.T) {
	assert := assert.New(t)

	upgraderConfig := &config.Upgrader{
		DisableUpdateCheck: false,
	}
	alertMgr := &alertmanager.AlertManager{}
	stateMgr := state.NewStateManager(fake.NewSimpleClientset(), "kwatch")

	u := NewUpgrader(upgraderConfig, alertMgr, stateMgr)
	assert.NotNil(u)
	assert.False(u.config.DisableUpdateCheck)
}

func TestUpgraderConfigDefaults(t *testing.T) {
	assert := assert.New(t)

	upgraderConfig := &config.Upgrader{}
	alertMgr := &alertmanager.AlertManager{}
	stateMgr := state.NewStateManager(fake.NewSimpleClientset(), "kwatch")

	u := NewUpgrader(upgraderConfig, alertMgr, stateMgr)
	assert.NotNil(u)
	assert.False(u.config.DisableUpdateCheck)
}

func TestUpgraderNilConfigNilStateManager(t *testing.T) {
	assert := assert.New(t)

	alertMgr := &alertmanager.AlertManager{}

	u := NewUpgrader(nil, alertMgr, nil)
	assert.NotNil(u)
	assert.Nil(u.config)
	assert.Nil(u.stateManager)
}

func TestUpgraderReuseStateManager(t *testing.T) {
	assert := assert.New(t)

	upgraderConfig := &config.Upgrader{}
	alertMgr := &alertmanager.AlertManager{}
	sharedStateMgr := state.NewStateManager(fake.NewSimpleClientset(), "kwatch")

	u1 := NewUpgrader(upgraderConfig, alertMgr, sharedStateMgr)
	u2 := NewUpgrader(upgraderConfig, alertMgr, sharedStateMgr)

	assert.Equal(u1.stateManager, u2.stateManager)
	assert.Equal(u1.stateManager, sharedStateMgr)
	assert.Equal(u2.stateManager, sharedStateMgr)
}

func TestUpgraderStateManager(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	stateMgr := state.NewStateManager(client, "kwatch")
	upgraderConfig := &config.Upgrader{}
	alertMgr := &alertmanager.AlertManager{}

	u := NewUpgrader(upgraderConfig, alertMgr, stateMgr)
	assert.NotNil(u)
	assert.Equal(stateMgr, u.stateManager)
}

func TestUpgraderGetNotifiedVersion(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	namespace := "kwatch"

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kwatch-state",
			Namespace: namespace,
		},
		Data: map[string]string{
			"notified-version": "v2.0.0",
		},
	}
	_, err := client.CoreV1().ConfigMaps(namespace).Create(
		context.Background(), cm, metav1.CreateOptions{})
	assert.Nil(err)

	stateMgr := state.NewStateManager(client, namespace)
	upgraderConfig := &config.Upgrader{}
	alertMgr := &alertmanager.AlertManager{}

	u := NewUpgrader(upgraderConfig, alertMgr, stateMgr)
	assert.NotNil(u)
	assert.Equal("v2.0.0", u.stateManager.GetNotifiedVersion(context.Background()))
}
