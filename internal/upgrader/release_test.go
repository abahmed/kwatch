package upgrader

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-github/v55/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/delivery"
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

	persistenceManager := &memoryVersions{version: newVersion}
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

	persistenceManager := &memoryVersions{}
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

	persistenceManager := &memoryVersions{}
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
