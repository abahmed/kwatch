package provider

import (
	"strings"

	"github.com/abahmed/kwatch/config"
	"github.com/abahmed/kwatch/event"
	"github.com/sirupsen/logrus"
)

const (
	footer       = "<https://github.com/abahmed/kwatch|kwatch>"
	defaultTitle = ":red_circle: kwatch detected a crash in pod"
	defaultText  = "There is an issue with container in a pod!"
)

// Provider interface
type Provider interface {
	Name() string
	SendEvent(*event.Event) error
	SendMessage(string) error
}

// GetProviders returns slice of provider objects after parsing config
func GetProviders(config *config.Config) []Provider {
	providers := make([]Provider, 0)

	if config.Alert == nil {
		return providers
	}

	for k, v := range config.Alert {
		lowerCaseKey := strings.ToLower(k)
		var pvdr func(map[string]string) Provider
		if lowerCaseKey == "slack" {
			pvdr = NewSlack
		} else if lowerCaseKey == "pagerduty" {
			pvdr = NewPagerDuty
		} else if lowerCaseKey == "discord" {
			pvdr = NewDiscord
		} else if lowerCaseKey == "telegram" {
			pvdr = NewTelegram
		} else if lowerCaseKey == "teams" {
			pvdr = NewTeams
		} else if lowerCaseKey == "email" {
			pvdr = NewEmail
		} else if lowerCaseKey == "rocketchat" {
			pvdr = NewRocketChat
		} else if lowerCaseKey == "mattermost" {
			pvdr = NewMattermost
		} else if lowerCaseKey == "opsgenie" {
			pvdr = NewOpsgenie
		}

		if pvdr != nil {
			instance := pvdr(v)
			if instance != nil {
				providers = append(
					providers, instance)
			}
		}
	}

	return providers
}

// SendProvidersMsg sends string msg to all providers
func SendProvidersMsg(p []Provider, msg string) {
	logrus.Infof("sending message: %s", msg)
	for _, prv := range p {
		err :=
			prv.SendMessage(msg)
		if err != nil {
			logrus.Errorf(
				"failed to send msg with %s: %s",
				prv.Name(),
				err.Error())
		}
	}
}

// SendProvidersEvent sends event to all providers
func SendProvidersEvent(p []Provider, event event.Event) {
	logrus.Infof("sending event: %+v", event)
	for _, prv := range p {
		if err := prv.SendEvent(&event); err != nil {
			logrus.Errorf(
				"failed to send event with %s: %s",
				prv.Name(),
				err.Error(),
			)
		}
	}
}
