package upgrader

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-github/v55/github"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/version"
)

type GitHubReleaseChecker interface {
	GetLatestRelease(
		ctx context.Context,
		owner, repo string,
	) (*github.RepositoryRelease, *github.Response, error)
}

type GitHubClient struct {
	client *http.Client
}

func (c *GitHubClient) GetLatestRelease(
	ctx context.Context,
	owner, repo string,
) (*github.RepositoryRelease, *github.Response, error) {
	if c.client == nil {
		return nil, nil, fmt.Errorf("github HTTP client is not configured")
	}
	client := github.NewClient(c.client)
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
	config             *config.Upgrader
	deliveryManager    Notifier
	persistenceManager VersionTracker
	githubClient       GitHubReleaseChecker
}

func NewUpgrader(
	upCfg *config.Upgrader,
	deliveryManager Notifier,
	persistenceManager VersionTracker,
	httpClient *http.Client,
) *Upgrader {
	if upCfg == nil {
		upCfg = &config.Upgrader{}
	}
	if os.Getenv("SKIP_UPGRADE_CHECK") == "1" ||
		os.Getenv("SKIP_UPGRADE_CHECK") == "true" {
		upCfg.DisableUpdateCheck = true
	}
	return &Upgrader{
		config:             upCfg,
		deliveryManager:    deliveryManager,
		persistenceManager: persistenceManager,
		githubClient:       &GitHubClient{client: httpClient},
	}
}

func (u *Upgrader) CheckUpdates(ctx context.Context) {
	if u.config.DisableUpdateCheck ||
		version.Short() == "dev" {
		if u.config.DisableUpdateCheck {
			klog.InfoS("update check disabled", "component", "upgrader")
		}
		return
	}

	if u.isPrerelease(version.Short()) {
		klog.InfoS(
			"prerelease build; skipping update check",
			"component",
			"upgrader",
			"version",
			version.Short(),
		)
		return
	}

	u.checkRelease(ctx)

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			klog.InfoS("upgrader stopped")
			return
		case <-ticker.C:
			u.checkRelease(ctx)
		}
	}
}

// isPrerelease reports whether the baked version string denotes a prerelease
// (e.g. v0.11.0-rc.1). RC builds opt into the dev channel, so their users are
// never nagged with notices for the latest stable release.
func (u *Upgrader) isPrerelease(version string) bool {
	return strings.Contains(version, "-rc")
}

func (u *Upgrader) checkRelease(ctx context.Context) {
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

	if !isNewer(*r.TagName, version.Short()) {
		return
	}

	if u.persistenceManager != nil {
		notifiedVersion := u.persistenceManager.GetNotifiedVersion(ctx)
		if notifiedVersion == *r.TagName {
			klog.V(4).InfoS(
				"already notified about version, skipping",
				"version", *r.TagName)
			return
		}
	}

	u.deliveryManager.Notify(fmt.Sprintf(constant.KwatchUpdateMsg, *r.TagName))

	if u.persistenceManager != nil {
		if err := u.persistenceManager.SetNotifiedVersion(
			ctx,
			*r.TagName,
		); err != nil {
			klog.InfoS("failed to set notified version", "error", err)
		}
	}
}

// parseSemver reads "v1.2.3" or "1.2.3" (an optional "-pre" suffix is
// tolerated and ignored). ok is false for anything else.
func parseSemver(v string) (parts [3]int, ok bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	fields := strings.Split(v, ".")
	if len(fields) != 3 {
		return parts, false
	}
	for i, f := range fields {
		n, err := strconv.Atoi(f)
		if err != nil || n < 0 {
			return parts, false
		}
		parts[i] = n
	}
	return parts, true
}

// isNewer reports whether latest is a strictly newer release than current.
// A plain string comparison would nag a v0.12.0 install about v0.9.5, and a
// build running ahead of the latest published release about that release.
// When either side is not a version at all, fall back to "different means
// newer" so an unusual tag still gets reported.
func isNewer(latest, current string) bool {
	l, lok := parseSemver(latest)
	c, cok := parseSemver(current)
	if !lok || !cok {
		return latest != current
	}
	for i := 0; i < 3; i++ {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}
