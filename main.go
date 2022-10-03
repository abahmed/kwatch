package main

import (
	"fmt"

	"github.com/abahmed/kwatch/alertmanager"
	"github.com/abahmed/kwatch/config"
	"github.com/abahmed/kwatch/constant"
	"github.com/abahmed/kwatch/controller"
	"github.com/abahmed/kwatch/upgrader"
	"github.com/sirupsen/logrus"
)

func main() {
	logrus.Infof(fmt.Sprintf(constant.WelcomeMsg, constant.Version))

	config, err := config.LoadConfig()
	if err != nil {
		logrus.Fatalf("failed to load config: %s", err.Error())
	}

	alertManager := alertmanager.AlertManager{}
	alertManager.Init(config.Alert)

	// check and notify if newer versions are available
	upgrader := upgrader.NewUpgrader(&config.Upgrader, &alertManager)
	go upgrader.CheckUpdates()

	// start controller
	controller.Start(
		&alertManager,
		config,
	)
}
