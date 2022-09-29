package main

import (
	"fmt"

	"github.com/abahmed/kwatch/alertmanager"
	"github.com/abahmed/kwatch/config"
	"github.com/abahmed/kwatch/constant"
	"github.com/abahmed/kwatch/controller"
	"github.com/abahmed/kwatch/event"
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

	alertManager.Notify("hello\nworld!!!!")
	alertManager.NotifyEvent(event.Event{
		Name:      "test-pod",
		Container: "test-container",
		Namespace: "default",
		Reason:    "OOMKILLED",
		Logs:      "test logs line 1\ntest logs line 2",
		Events: "event1-event2-event3-event4-event5" +
			"event6\nevent7\nevent8-event9",
	})
	return

	// check and notify if newer versions are available
	go upgrader.CheckUpdates(&config.Upgrader, &alertManager)

	// start controller
	controller.Start(
		&alertManager,
		config,
	)
}
