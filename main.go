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

	alertManager.NotifyEvent(event.Event{
		Name:      "jb6e58fc08-ea2b-46ea-bb87-6123a4e94bcc-d6hxz",
		Container: "jb6e58fc08-ea2b-46ea-bb87-6123a4e94bcc",
		Namespace: "production",
		Reason:    "OOMKILLED",
		Events:    "[2022-09-29 022042 +0000 UTC] SuccessfulAttachVolume AttachVolume.Attach succeeded for volume \"pvc-19096777-35bf-4d27-bf5f-ace40da5a49f\"\n[2022-09-29 022045 +0000 UTC] Created Created container jb6e58fc08-ea2b-46ea-bb87-6123a4e94bcc",
		Logs:      "previous terminated container \"jb6e58fc08-ea2b-46ea-bb87-6123a4e94bcc\" in pod \"jb6e58fc08-ea2b-46ea-bb87-6123a4e94bcc-d6hxz\" not found",
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
