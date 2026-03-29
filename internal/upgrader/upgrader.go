package upgrader

import (
	"context"
	"fmt"
	"time"

	"github.com/abahmed/kwatch/internal/alert"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/state"
	"github.com/abahmed/kwatch/internal/version"
	"github.com/google/go-github/v41/github"
	"k8s.io/klog/v2"
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
	alertManager *alert.AlertManager,
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
		klog.InfoS("failed to get latest release", "error", err.Error())
		return
	}

	if r.TagName == nil {
		klog.InfoS("failed to get release tag", "release", r)
		return
	}

	if version.Short() == *r.TagName {
		return
	}

	if u.stateManager != nil {
		notifiedVersion := u.stateManager.GetNotifiedVersion(ctx)
		if notifiedVersion == *r.TagName {
			klog.V(4).InfoS(
				"already notified about version, skipping",
				"version", *r.TagName)
			return
		}
	}

	u.alertManager.Notify(fmt.Sprintf(constant.KwatchUpdateMsg, *r.TagName))

	if u.stateManager != nil {
		if err := u.stateManager.SetNotifiedVersion(ctx, *r.TagName); err != nil {
			klog.InfoS("failed to set notified version", "error", err)
		}
	}
}
