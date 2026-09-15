package upgrader

import (
	"context"
	"errors"
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

func TestCheckReleaseGitHubError(t *testing.T) {
	mockGithub := new(MockGitHubClient)
	mockGithub.On("GetLatestRelease", mock.Anything, "abahmed", "kwatch").
		Return(nil, nil, errors.New("rate limit exceeded"))

	u := newTestUpgrader(&config.Upgrader{}, &delivery.Manager{}, nil)
	u.githubClient = mockGithub

	u.checkRelease(context.Background())

	mockGithub.AssertExpectations(t)
}

func TestCheckReleaseNilTagName(t *testing.T) {
	mockGithub := new(MockGitHubClient)
	mockGithub.On("GetLatestRelease", mock.Anything, "abahmed", "kwatch").
		Return(&github.RepositoryRelease{}, nil, nil)

	u := newTestUpgrader(&config.Upgrader{}, &delivery.Manager{}, nil)
	u.githubClient = mockGithub

	u.checkRelease(context.Background())

	mockGithub.AssertExpectations(t)
}

func TestCheckReleaseSameVersion(t *testing.T) {
	mockGithub := new(MockGitHubClient)
	currentVersion := version.Short()
	mockGithub.On("GetLatestRelease", mock.Anything, "abahmed", "kwatch").
		Return(&github.RepositoryRelease{TagName: &currentVersion}, nil, nil)

	u := newTestUpgrader(&config.Upgrader{}, &delivery.Manager{}, nil)
	u.githubClient = mockGithub

	u.checkRelease(context.Background())

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
			Name: "kwatch-state", Namespace: "kwatch",
		},
		Data: map[string]string{"notified-version": newVersion},
	}
	_, err := client.CoreV1().ConfigMaps("kwatch").Create(
		context.Background(), cm, metav1.CreateOptions{},
	)
	assert.Nil(t, err)

	persistenceManager := persistence.NewManager(client, "kwatch")
	u := newTestUpgrader(
		&config.Upgrader{}, &delivery.Manager{}, persistenceManager,
	)
	u.githubClient = mockGithub

	u.checkRelease(context.Background())

	mockGithub.AssertExpectations(t)
}

func TestCheckReleaseNewVersionNotifies(t *testing.T) {
	newVersion := "v99.0.0"
	mockGithub := new(MockGitHubClient)
	mockGithub.On("GetLatestRelease", mock.Anything, "abahmed", "kwatch").
		Return(&github.RepositoryRelease{TagName: &newVersion}, nil, nil)

	notifier := new(recordingNotifier)
	notifier.On("Notify", mock.AnythingOfType("string")).Return()

	persistenceManager := persistence.NewManager(
		fake.NewSimpleClientset(), "kwatch",
	)
	u := newTestUpgrader(
		&config.Upgrader{}, &delivery.Manager{}, persistenceManager,
	)
	u.githubClient = mockGithub
	u.deliveryManager = notifier

	u.checkRelease(context.Background())

	mockGithub.AssertExpectations(t)
	notifier.AssertExpectations(t)
	assert.True(t, notifier.NotifyCalled)
}

func TestCheckReleaseNewVersionSetsState(t *testing.T) {
	newVersion := "v99.0.0"
	mockGithub := new(MockGitHubClient)
	mockGithub.On("GetLatestRelease", mock.Anything, "abahmed", "kwatch").
		Return(&github.RepositoryRelease{TagName: &newVersion}, nil, nil)

	notifier := new(recordingNotifier)
	notifier.On("Notify", mock.AnythingOfType("string")).Return()

	client := fake.NewSimpleClientset()
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kwatch-state", Namespace: "kwatch",
		},
		Data: map[string]string{},
	}
	_, err := client.CoreV1().ConfigMaps("kwatch").Create(
		context.Background(), cm, metav1.CreateOptions{},
	)
	assert.Nil(t, err)

	persistenceManager := persistence.NewManager(client, "kwatch")
	u := newTestUpgrader(
		&config.Upgrader{}, &delivery.Manager{}, persistenceManager,
	)
	u.githubClient = mockGithub
	u.deliveryManager = notifier

	u.checkRelease(context.Background())

	mockGithub.AssertExpectations(t)
	notifier.AssertExpectations(t)
	assert.True(t, notifier.NotifyCalled)

	notifiedVersion := persistenceManager.GetNotifiedVersion(context.Background())
	assert.Equal(t, newVersion, notifiedVersion)
}
