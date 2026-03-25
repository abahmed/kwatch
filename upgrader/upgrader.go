package upgrader

import (
	"context"
	"fmt"
	"time"

	"github.com/abahmed/kwatch/alertmanager"
	"github.com/abahmed/kwatch/config"
	"github.com/abahmed/kwatch/constant"
	"github.com/abahmed/kwatch/state"
	"github.com/abahmed/kwatch/version"
	"github.com/google/go-github/v41/github"
	"github.com/sirupsen/logrus"
)

type GitHubReleaseChecker interface {
	GetLatestRelease(ctx context.Context, owner, repo string) (*github.RepositoryRelease, *github.Response, error)
}

type GitHubClient struct{}

func (c *GitHubClient) GetLatestRelease(ctx context.Context, owner, repo string) (*github.RepositoryRelease, *github.Response, error) {
	client := github.NewClient(nil)
	return client.Repositories.GetLatestRelease(ctx, owner, repo)
}

type Notifier interface {
	Notify(msg string)
}

type VersionTracker interface {
	GetNotifiedVersion(ctx context.Context) string
	SetNotifiedVersion(ctx context.Context, version string) error
}

type Upgrader struct {
	config       *config.Upgrader
	alertManager Notifier
	stateManager VersionTracker
	githubClient GitHubReleaseChecker
}

func NewUpgrader(
	config *config.Upgrader,
	alertManager *alertmanager.AlertManager,
	stateManager *state.StateManager,
) *Upgrader {
	return &Upgrader{
		config:       config,
		alertManager: alertManager,
		stateManager: stateManager,
		githubClient: &GitHubClient{},
	}
}

func (u *Upgrader) SetGitHubClient(client GitHubReleaseChecker) {
	u.githubClient = client
}

func (u *Upgrader) SetAlertManager(alertMgr Notifier) {
	u.alertManager = alertMgr
}

func (u *Upgrader) SetStateManager(stateMgr VersionTracker) {
	u.stateManager = stateMgr
}

func (u *Upgrader) CheckUpdates() {
	if u.config.DisableUpdateCheck ||
		version.Short() == "dev" {
		return
	}

	u.checkRelease()

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		u.checkRelease()
	}
}

func (u *Upgrader) checkRelease() {
	ctx := context.Background()

	r, _, err := u.githubClient.GetLatestRelease(
		ctx,
		"abahmed",
		"kwatch")
	if err != nil {
		logrus.Warnf("failed to get latest release: %s", err.Error())
		return
	}

	if r.TagName == nil {
		logrus.Warnf("failed to get release tag: %+v", r)
		return
	}

	if version.Short() == *r.TagName {
		return
	}

	if u.stateManager != nil {
		notifiedVersion := u.stateManager.GetNotifiedVersion(ctx)
		if notifiedVersion == *r.TagName {
			logrus.Debugf(
				"already notified about version %s, skipping",
				*r.TagName)
			return
		}
	}

	u.alertManager.Notify(fmt.Sprintf(constant.KwatchUpdateMsg, *r.TagName))

	if u.stateManager != nil {
		if err := u.stateManager.SetNotifiedVersion(ctx, *r.TagName); err != nil {
			logrus.Warnf("failed to set notified version: %v", err)
		}
	}
}
