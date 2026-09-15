package upgrader

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/go-github/v55/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/delivery"
	"github.com/abahmed/kwatch/internal/persistence"
	"github.com/abahmed/kwatch/internal/version"
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

type recordingNotifier struct {
	mock.Mock
	NotifyCalled  bool
	NotifyLastMsg string
}

func (m *recordingNotifier) Notify(msg string) {
	m.NotifyCalled = true
	m.NotifyLastMsg = msg
	m.Called(msg)
}

func newTestUpgrader(
	upgraderConfig *config.Upgrader,
	deliveryManager *delivery.Manager,
	persistenceManager *persistence.Manager,
) *Upgrader {
	return NewUpgrader(
		upgraderConfig,
		deliveryManager,
		persistenceManager,
		http.DefaultClient,
	)
}

func TestNewUpgrader(t *testing.T) {
	assert := assert.New(t)

	upgraderConfig := &config.Upgrader{}
	deliveryManager := &delivery.Manager{}
	persistenceManager := persistence.NewManager(
		fake.NewSimpleClientset(),
		"kwatch",
	)

	u := newTestUpgrader(upgraderConfig, deliveryManager, persistenceManager)
	assert.NotNil(u)
	assert.Equal(upgraderConfig, u.config)
	assert.Equal(deliveryManager, u.deliveryManager)
	assert.Equal(persistenceManager, u.persistenceManager)
}

func TestNewUpgraderNilPersistenceManager(t *testing.T) {
	assert := assert.New(t)

	upgraderConfig := &config.Upgrader{}
	deliveryManager := &delivery.Manager{}

	u := newTestUpgrader(upgraderConfig, deliveryManager, nil)
	assert.NotNil(u)
	assert.Nil(u.persistenceManager)
}

func TestCheckUpdatesDisabled(t *testing.T) {
	assert := assert.New(t)

	upgraderConfig := &config.Upgrader{
		DisableUpdateCheck: true,
	}
	deliveryManager := &delivery.Manager{}
	persistenceManager := persistence.NewManager(
		fake.NewSimpleClientset(),
		"kwatch",
	)

	u := newTestUpgrader(upgraderConfig, deliveryManager, persistenceManager)
	assert.NotNil(u)
}

func TestUpgraderFields(t *testing.T) {
	assert := assert.New(t)

	upgraderConfig := &config.Upgrader{
		DisableUpdateCheck: true,
	}
	deliveryManager := &delivery.Manager{}
	persistenceManager := persistence.NewManager(
		fake.NewSimpleClientset(),
		"kwatch",
	)

	u := newTestUpgrader(upgraderConfig, deliveryManager, persistenceManager)
	assert.NotNil(u)
	assert.Equal(upgraderConfig, u.config)
	assert.Equal(deliveryManager, u.deliveryManager)
	assert.Equal(persistenceManager, u.persistenceManager)
	assert.True(u.config.DisableUpdateCheck)
}

func TestVersionComparison(t *testing.T) {
	assert := assert.New(t)

	currentVersion := version.Short()
	assert.NotEmpty(currentVersion)
}

func TestIsPrerelease(t *testing.T) {
	assert := assert.New(t)

	u := &Upgrader{}
	assert.True(u.isPrerelease("v0.11.0-rc.1"))
	assert.True(u.isPrerelease("v0.11.0-rc.17"))
	assert.False(u.isPrerelease("v0.10.5"))
	assert.False(u.isPrerelease("v0.11.0"))
	assert.False(u.isPrerelease("dev"))
}

func TestUpgraderWithDisabledConfig(t *testing.T) {
	assert := assert.New(t)

	upgraderConfig := &config.Upgrader{
		DisableUpdateCheck: true,
	}
	deliveryManager := &delivery.Manager{}
	persistenceManager := persistence.NewManager(
		fake.NewSimpleClientset(),
		"kwatch",
	)

	u := newTestUpgrader(upgraderConfig, deliveryManager, persistenceManager)
	assert.NotNil(u)
	assert.True(u.config.DisableUpdateCheck)
}

func TestUpgraderWithEnabledConfig(t *testing.T) {
	assert := assert.New(t)

	upgraderConfig := &config.Upgrader{
		DisableUpdateCheck: false,
	}
	deliveryManager := &delivery.Manager{}
	persistenceManager := persistence.NewManager(
		fake.NewSimpleClientset(),
		"kwatch",
	)

	u := newTestUpgrader(upgraderConfig, deliveryManager, persistenceManager)
	assert.NotNil(u)
	assert.False(u.config.DisableUpdateCheck)
}

func TestUpgraderConfigDefaults(t *testing.T) {
	assert := assert.New(t)

	upgraderConfig := &config.Upgrader{}
	deliveryManager := &delivery.Manager{}
	persistenceManager := persistence.NewManager(
		fake.NewSimpleClientset(),
		"kwatch",
	)

	u := newTestUpgrader(upgraderConfig, deliveryManager, persistenceManager)
	assert.NotNil(u)
	assert.False(u.config.DisableUpdateCheck)
}

func TestUpgraderNilConfigNilPersistenceManager(t *testing.T) {
	assert := assert.New(t)

	deliveryManager := &delivery.Manager{}

	u := newTestUpgrader(nil, deliveryManager, nil)
	assert.NotNil(u)
	assert.NotNil(u.config)
	assert.Nil(u.persistenceManager)
}

func TestUpgraderReusePersistenceManager(t *testing.T) {
	assert := assert.New(t)

	upgraderConfig := &config.Upgrader{}
	deliveryManager := &delivery.Manager{}
	sharedPersistenceManager := persistence.NewManager(
		fake.NewSimpleClientset(),
		"kwatch",
	)

	u1 := newTestUpgrader(
		upgraderConfig, deliveryManager, sharedPersistenceManager,
	)
	u2 := newTestUpgrader(
		upgraderConfig, deliveryManager, sharedPersistenceManager,
	)

	assert.Equal(u1.persistenceManager, u2.persistenceManager)
	assert.Equal(u1.persistenceManager, sharedPersistenceManager)
	assert.Equal(u2.persistenceManager, sharedPersistenceManager)
}

func TestUpgraderPersistenceManager(t *testing.T) {
	assert := assert.New(t)

	client := fake.NewSimpleClientset()
	persistenceManager := persistence.NewManager(client, "kwatch")
	upgraderConfig := &config.Upgrader{}
	deliveryManager := &delivery.Manager{}

	u := newTestUpgrader(upgraderConfig, deliveryManager, persistenceManager)
	assert.NotNil(u)
	assert.Equal(persistenceManager, u.persistenceManager)
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

	persistenceManager := persistence.NewManager(client, namespace)
	upgraderConfig := &config.Upgrader{}
	deliveryManager := &delivery.Manager{}

	u := newTestUpgrader(upgraderConfig, deliveryManager, persistenceManager)
	assert.NotNil(u)
	assert.Equal(
		"v2.0.0",
		u.persistenceManager.GetNotifiedVersion(context.Background()),
	)
}
