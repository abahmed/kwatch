package upgrader

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/go-github/v55/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/delivery"
	"github.com/abahmed/kwatch/internal/notification"
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
	u.current = "v0.10.5"

	u.checkRelease(context.Background())

	mockGithub.AssertExpectations(t)
	notifier.AssertExpectations(t)
	assert.True(t, notifier.NotifyCalled)
	assert.Equal(t, "🟡 kwatch v99.0.0 is available; this cluster runs "+
		"v0.10.5.", notifier.NotifyLastMsg)
}

// The upgrade notice is one plain sentence with one leading marker and
// no links or shortcodes.
func TestUpdateNoticeIsOnePlainSentence(t *testing.T) {
	got := updateNotice("v1.3.0", "v1.2.0")
	assert.Equal(t,
		"🟡 kwatch v1.3.0 is available; this cluster runs v1.2.0.", got)
	n := notification.Notice(got)
	assert.Equal(t, notification.MarkerLow, n.Marker)
	assert.Equal(t, notification.StatusLow, n.Status)
	markers := 0
	for _, m := range notification.Markers() {
		markers += strings.Count(got, m)
	}
	assert.Equal(t, 1, markers)
	for _, bad := range []string{"http", "<", ":tada:", "\n"} {
		assert.NotContains(t, got, bad)
	}
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
	u.current = "v0.10.5"

	u.checkRelease(context.Background())

	mockGithub.AssertExpectations(t)
	notifier.AssertExpectations(t)
	assert.True(t, notifier.NotifyCalled)

	notifiedVersion := persistenceManager.GetNotifiedVersion(context.Background())
	assert.Equal(t, newVersion, notifiedVersion)
}

func TestCheckReleaseWithoutPersistenceNotifiesOnce(t *testing.T) {
	newVersion := "v99.0.0"
	mockGithub := new(MockGitHubClient)
	mockGithub.On("GetLatestRelease", mock.Anything, "abahmed", "kwatch").
		Return(&github.RepositoryRelease{TagName: &newVersion}, nil, nil)

	notifier := new(recordingNotifier)
	notifier.On("Notify", mock.AnythingOfType("string")).Return()

	u := &Upgrader{
		config:          &config.Upgrader{},
		githubClient:    mockGithub,
		deliveryManager: notifier,
		current:         "v0.10.5",
	}

	u.checkRelease(context.Background())
	u.checkRelease(context.Background())

	notifier.AssertNumberOfCalls(t, "Notify", 1)
}

func TestNewUpgraderDoesNotMutateCallerConfig(t *testing.T) {
	t.Setenv("SKIP_UPGRADE_CHECK", "true")
	cfg := &config.Upgrader{}

	u := NewUpgrader(cfg, nil, nil, nil)

	assert.False(t, cfg.DisableUpdateCheck)
	assert.True(t, u.config.DisableUpdateCheck)
}

func TestSkipUpgradeCheckUsesBooleanParsing(t *testing.T) {
	for value, want := range map[string]bool{
		"yes": true, "ON": true, "1": true, "no": false,
		"": false, "typo": false,
	} {
		t.Setenv("SKIP_UPGRADE_CHECK", value)

		u := NewUpgrader(&config.Upgrader{}, nil, nil, nil)

		assert.Equal(t, want, u.config.DisableUpdateCheck, value)
	}
}

// A build that is not a release (dev, e2e) must not announce a release,
// which would be older than the build.
func TestCheckReleaseFromNonReleaseBuildStaysQuiet(t *testing.T) {
	for _, current := range []string{"e2e", "dev", ""} {
		t.Run(current, func(t *testing.T) {
			newVersion := "v0.10.5"
			mockGithub := new(MockGitHubClient)
			mockGithub.On("GetLatestRelease", mock.Anything, "abahmed",
				"kwatch").Return(&github.RepositoryRelease{
				TagName: &newVersion}, nil, nil)
			notifier := new(recordingNotifier)
			u := &Upgrader{config: &config.Upgrader{},
				githubClient: mockGithub, deliveryManager: notifier,
				current: current}

			u.checkRelease(context.Background())

			assert.False(t, notifier.NotifyCalled)
		})
	}
}
