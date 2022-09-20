package upgrader

import (
	"context"
	"fmt"
	"time"

	"github.com/abahmed/kwatch/constant"
	"github.com/abahmed/kwatch/provider"
	"github.com/google/go-github/v41/github"
	"github.com/sirupsen/logrus"
)

// CheckUpdates checks every 24 hours if a newer version of Kwatch is available
func CheckUpdates(providers []provider.Provider) {
	// check at startup
	checkRelease(providers)

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		checkRelease(providers)
	}
}

func checkRelease(providers []provider.Provider) {
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

	provider.SendProvidersMsg(
		providers,
		fmt.Sprintf(constant.KwatchUpdateMsg, *r.TagName),
	)
}
