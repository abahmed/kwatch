package upgrader

import (
	"context"
	"errors"
	"testing"

	"github.com/abahmed/kwatch/alertmanager"
	"github.com/abahmed/kwatch/config"
	"github.com/abahmed/kwatch/state"
	"github.com/abahmed/kwatch/version"
	"github.com/google/go-github/v41/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

type MockGitHubClient struct {
	mock.Mock
}

func (m *MockGitHubClient) GetLatestRelease(ctx context.Context, owner, repo string) (*github.RepositoryRelease, *github.Response, error) {
	args := m.Called(ctx, owner, repo)
	var r0 *github.RepositoryRelease
	var r1 *github.Response
	if args.Get(0) != nil {
		r0 = args.Get(0).(*github.RepositoryRelease)
	}
	if args.Get(1) != nil {
		r1 = args.Get(1).(*github.Response)
	}
	return r0, r1, args.Error(2)
}

type MockAlertManager struct {
	mock.Mock
	NotifyCalled  bool
	NotifyLastMsg string
}

func (m *MockAlertManager) Notify(msg string) {
	m.NotifyCalled = true
	m.NotifyLastMsg = msg
	m.Called(msg)
}

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

func TestCheckReleaseGitHubError(t *testing.T) {
	mockGithub := new(MockGitHubClient)
	mockGithub.On("GetLatestRelease", mock.Anything, "abahmed", "kwatch").
		Return(nil, nil, errors.New("rate limit exceeded"))

	u := NewUpgrader(&config.Upgrader{}, &alertmanager.AlertManager{}, nil)
	u.SetGitHubClient(mockGithub)

	u.checkRelease()

	mockGithub.AssertExpectations(t)
}

func TestCheckReleaseNilTagName(t *testing.T) {
	mockGithub := new(MockGitHubClient)
	mockGithub.On("GetLatestRelease", mock.Anything, "abahmed", "kwatch").
		Return(&github.RepositoryRelease{}, nil, nil)

	u := NewUpgrader(&config.Upgrader{}, &alertmanager.AlertManager{}, nil)
	u.SetGitHubClient(mockGithub)

	u.checkRelease()

	mockGithub.AssertExpectations(t)
}

func TestCheckReleaseSameVersion(t *testing.T) {
	mockGithub := new(MockGitHubClient)
	currentVersion := version.Short()
	mockGithub.On("GetLatestRelease", mock.Anything, "abahmed", "kwatch").
		Return(&github.RepositoryRelease{TagName: &currentVersion}, nil, nil)

	u := NewUpgrader(&config.Upgrader{}, &alertmanager.AlertManager{}, nil)
	u.SetGitHubClient(mockGithub)

	u.checkRelease()

	mockGithub.AssertExpectations(t)
}

func TestCheckReleaseAlreadyNotified(t *testing.T) {
	newVersion := "v99.0.0"
	mockGithub := new(MockGitHubClient)
	mockGithub.On("GetLatestRelease", mock.Anything, "abahmed", "kwatch").
		Return(&github.RepositoryRelease{TagName: &newVersion}, nil, nil)

	client := fake.NewSimpleClientset()
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kwatch-state",
			Namespace: "kwatch",
		},
		Data: map[string]string{
			"notified-version": newVersion,
		},
	}
	_, err := client.CoreV1().ConfigMaps("kwatch").Create(context.Background(), cm, metav1.CreateOptions{})
	assert.Nil(t, err)

	stateMgr := state.NewStateManager(client, "kwatch")

	u := NewUpgrader(&config.Upgrader{}, &alertmanager.AlertManager{}, stateMgr)
	u.SetGitHubClient(mockGithub)

	u.checkRelease()

	mockGithub.AssertExpectations(t)
}

func TestCheckReleaseNewVersionNotifies(t *testing.T) {
	newVersion := "v99.0.0"
	mockGithub := new(MockGitHubClient)
	mockGithub.On("GetLatestRelease", mock.Anything, "abahmed", "kwatch").
		Return(&github.RepositoryRelease{TagName: &newVersion}, nil, nil)

	mockAlert := new(MockAlertManager)
	mockAlert.On("Notify", mock.AnythingOfType("string")).Return()

	stateMgr := state.NewStateManager(fake.NewSimpleClientset(), "kwatch")

	u := NewUpgrader(&config.Upgrader{}, &alertmanager.AlertManager{}, stateMgr)
	u.SetGitHubClient(mockGithub)
	u.SetAlertManager(mockAlert)

	u.checkRelease()

	mockGithub.AssertExpectations(t)
	mockAlert.AssertExpectations(t)
	assert.True(t, mockAlert.NotifyCalled)
}

func TestCheckReleaseNewVersionSetsState(t *testing.T) {
	newVersion := "v99.0.0"
	mockGithub := new(MockGitHubClient)
	mockGithub.On("GetLatestRelease", mock.Anything, "abahmed", "kwatch").
		Return(&github.RepositoryRelease{TagName: &newVersion}, nil, nil)

	mockAlert := new(MockAlertManager)
	mockAlert.On("Notify", mock.AnythingOfType("string")).Return()

	client := fake.NewSimpleClientset()
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kwatch-state",
			Namespace: "kwatch",
		},
		Data: map[string]string{},
	}
	_, err := client.CoreV1().ConfigMaps("kwatch").Create(context.Background(), cm, metav1.CreateOptions{})
	assert.Nil(t, err)

	stateMgr := state.NewStateManager(client, "kwatch")

	u := NewUpgrader(&config.Upgrader{}, &alertmanager.AlertManager{}, stateMgr)
	u.SetGitHubClient(mockGithub)
	u.SetAlertManager(mockAlert)

	u.checkRelease()

	mockGithub.AssertExpectations(t)
	mockAlert.AssertExpectations(t)
	assert.True(t, mockAlert.NotifyCalled)

	notifiedVersion := stateMgr.GetNotifiedVersion(context.Background())
	assert.Equal(t, newVersion, notifiedVersion)
}
