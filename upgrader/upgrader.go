package upgrader

import (
	"context"
	"fmt"
	"time"

	"github.com/abahmed/kwatch/alertmanager"
	"github.com/abahmed/kwatch/config"
	"github.com/abahmed/kwatch/constant"
	"github.com/abahmed/kwatch/version"
	"github.com/google/go-github/v41/github"
	"github.com/sirupsen/logrus"
)

type Upgrader struct {
	config       *config.Upgrader
	alertManager *alertmanager.AlertManager
}

// NewUpgrader returns new instance of upgrader
func NewUpgrader(config *config.Upgrader,
	alertManager *alertmanager.AlertManager) *Upgrader {
	return &Upgrader{
		config:       config,
		alertManager: alertManager,
	}
}

// CheckUpdates checks every 24 hours if a newer version of Kwatch is available
func (u *Upgrader) CheckUpdates() {
	if u.config.DisableUpdateCheck ||
		version.Short() == "dev" {
		return
	}

	// check at startup
	u.checkRelease()

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		u.checkRelease()
	}
}

func (u *Upgrader) checkRelease() {
	client := github.NewClient(nil)

	r, _, err := client.Repositories.GetLatestRelease(
		context.TODO(),
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

	u.alertManager.Notify(fmt.Sprintf(constant.KwatchUpdateMsg, *r.TagName))
}
