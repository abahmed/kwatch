package upgrader

import (
	"context"
	"fmt"
	"time"

	"github.com/abahmed/kwatch/alertmanager"
	"github.com/abahmed/kwatch/config"
	"github.com/abahmed/kwatch/constant"
	"github.com/google/go-github/v41/github"
	"github.com/sirupsen/logrus"
)

// CheckUpdates checks every 24 hours if a newer version of Kwatch is available
func CheckUpdates(
	config *config.Upgrader,
	alertManager *alertmanager.AlertManager) {
	if config.DisableUpdateCheck {
		return
	}

	// check at startup
	checkRelease(alertManager)

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		checkRelease(alertManager)
	}
}

func checkRelease(alertManager *alertmanager.AlertManager) {
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

	if constant.Version == *r.TagName {
		return
	}

	alertManager.Notify(fmt.Sprintf(constant.KwatchUpdateMsg, *r.TagName))
}
