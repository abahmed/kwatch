package alertmanager

import (
	"strings"

	"github.com/abahmed/kwatch/alertmanager/discord"
	"github.com/abahmed/kwatch/alertmanager/email"
	"github.com/abahmed/kwatch/alertmanager/mattermost"
	"github.com/abahmed/kwatch/alertmanager/opsgenie"
	"github.com/abahmed/kwatch/alertmanager/pagerduty"
	"github.com/abahmed/kwatch/alertmanager/rocketchat"
	"github.com/abahmed/kwatch/alertmanager/slack"
	"github.com/abahmed/kwatch/alertmanager/teams"
	"github.com/abahmed/kwatch/alertmanager/telegram"
	"github.com/abahmed/kwatch/event"
	"github.com/sirupsen/logrus"
)

type AlertManager struct {
	providers []Provider
}

// Provider interface
type Provider interface {
	Name() string
	SendEvent(*event.Event) error
	SendMessage(string) error
}

// Init initializes AlertManager with provided config
func (a *AlertManager) Init(config map[string]map[string]string) {
	a.providers = make([]Provider, 0)

	for k, v := range config {
		lowerCaseKey := strings.ToLower(k)
		var pvdr Provider
		if lowerCaseKey == "slack" {
			pvdr = slack.NewSlack(v)
		} else if lowerCaseKey == "pagerduty" {
			pvdr = pagerduty.NewPagerDuty(v)
		} else if lowerCaseKey == "discord" {
			pvdr = discord.NewDiscord(v)
		} else if lowerCaseKey == "telegram" {
			pvdr = telegram.NewTelegram(v)
		} else if lowerCaseKey == "teams" {
			pvdr = teams.NewTeams(v)
		} else if lowerCaseKey == "email" {
			pvdr = email.NewEmail(v)
		} else if lowerCaseKey == "rocketchat" {
			pvdr = rocketchat.NewRocketChat(v)
		} else if lowerCaseKey == "mattermost" {
			pvdr = mattermost.NewMattermost(v)
		} else if lowerCaseKey == "opsgenie" {
			pvdr = opsgenie.NewOpsgenie(v)
		}

		if pvdr != nil {
			a.providers = append(a.providers, pvdr)

		}
	}
}

// Notify sends string msg to all providers
func (a *AlertManager) Notify(msg string) {
	logrus.Infof("sending message: %s", msg)

	for _, prv := range a.providers {
		if err := prv.SendMessage(msg); err != nil {
			logrus.Errorf(
				"failed to send msg with %s: %s",
				prv.Name(),
				err.Error())
		}
	}
}

// NotifyEvent sends event to all providers
func (a *AlertManager) NotifyEvent(event event.Event) {
	logrus.Infof("sending event: %+v", event)

	for _, prv := range a.providers {
		if err := prv.SendEvent(&event); err != nil {
			logrus.Errorf(
				"failed to send event with %s: %s",
				prv.Name(),
				err.Error(),
			)
		}
	}
}
