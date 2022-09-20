package main

import (
	"fmt"

	"github.com/abahmed/kwatch/config"
	"github.com/abahmed/kwatch/constant"
	"github.com/abahmed/kwatch/controller"
	"github.com/abahmed/kwatch/provider"
	"github.com/abahmed/kwatch/upgrader"
	"github.com/sirupsen/logrus"
)

func main() {
	logrus.Infof(fmt.Sprintf(constant.WelcomeMsg, constant.Version))

	config, err := config.LoadConfig()
	if err != nil {
		logrus.Fatalf("failed to load config: %s", err.Error())
	}

	// get providers
	providers := provider.GetProviders(config)

	// check and notify if newer versions are available
	if !config.DisableUpdateCheck {
		go upgrader.CheckUpdates(providers)
	}

	// start controller
	controller.Start(
		providers,
		config,
	)
}
