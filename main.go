package main

import (
	"fmt"

	"github.com/abahmed/kwatch/alertmanager"
	"github.com/abahmed/kwatch/client"
	"github.com/abahmed/kwatch/config"
	"github.com/abahmed/kwatch/constant"
	"github.com/abahmed/kwatch/handler"
	"github.com/abahmed/kwatch/pvcmonitor"
	"github.com/abahmed/kwatch/storage/memory"
	"github.com/abahmed/kwatch/upgrader"
	"github.com/abahmed/kwatch/version"
	"github.com/abahmed/kwatch/watcher"
	"github.com/sirupsen/logrus"
)

func main() {
	config, err := config.LoadConfig()
	if err != nil {
		logrus.Fatalf("failed to load config: %s", err.Error())
	}
	setLogFormatter(config.App.LogFormatter)

	logrus.Info(fmt.Sprintf(constant.WelcomeMsg, version.Short()))

	// create kubernetes client
	client := client.Create(&config.App)

	alertManager := alertmanager.AlertManager{}
	alertManager.Init(config.Alert, &config.App)

	if !config.App.DisableStartupMessage {
		// send notification to providers
		alertManager.Notify(fmt.Sprintf(constant.WelcomeMsg, version.Short()))
	}

	// check and notify if newer versions are available
	upgrader := upgrader.NewUpgrader(&config.Upgrader, &alertManager)
	go upgrader.CheckUpdates()

	// start monitoring Persistent Volume Claims
	pvcMonitor :=
		pvcmonitor.NewPvcMonitor(client, &config.PvcMonitor, &alertManager)
	go pvcMonitor.Start()

	// Create handler
	h := handler.NewHandler(
		client,
		config,
		memory.NewMemory(),
		&alertManager,
	)

	// start watcher
	watcher.Start(client, config, h)
}

func setLogFormatter(formatter string) {
	switch formatter {
	case "json":
		logrus.SetFormatter(&logrus.JSONFormatter{})
	default:
		logrus.SetFormatter(&logrus.TextFormatter{})
	}
}
