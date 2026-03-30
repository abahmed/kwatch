package alert

import (
	"reflect"
	"strings"

	"github.com/abahmed/kwatch/internal/alert/dingtalk"
	"github.com/abahmed/kwatch/internal/alert/discord"
	"github.com/abahmed/kwatch/internal/alert/email"
	"github.com/abahmed/kwatch/internal/alert/feishu"
	"github.com/abahmed/kwatch/internal/alert/googlechat"
	"github.com/abahmed/kwatch/internal/alert/matrix"
	"github.com/abahmed/kwatch/internal/alert/mattermost"
	"github.com/abahmed/kwatch/internal/alert/opsgenie"
	"github.com/abahmed/kwatch/internal/alert/pagerduty"
	"github.com/abahmed/kwatch/internal/alert/rocketchat"
	"github.com/abahmed/kwatch/internal/alert/slack"
	"github.com/abahmed/kwatch/internal/alert/teams"
	"github.com/abahmed/kwatch/internal/alert/telegram"
	"github.com/abahmed/kwatch/internal/alert/webhook"
	"github.com/abahmed/kwatch/internal/alert/zenduty"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
	"k8s.io/klog/v2"
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
func (a *AlertManager) Init(
	alertCfg map[string]map[string]interface{},
	appCfg *config.App) {
	a.providers = make([]Provider, 0)
	for k, v := range alertCfg {
		lowerCaseKey := strings.ToLower(k)
		var pvdr Provider = nil
		if lowerCaseKey == "slack" {
			pvdr = slack.NewSlack(v, appCfg)
		} else if lowerCaseKey == "pagerduty" {
			pvdr = pagerduty.NewPagerDuty(v, appCfg)
		} else if lowerCaseKey == "discord" {
			pvdr = discord.NewDiscord(v, appCfg)
		} else if lowerCaseKey == "telegram" {
			pvdr = telegram.NewTelegram(v, appCfg)
		} else if lowerCaseKey == "teams" {
			pvdr = teams.NewTeams(v, appCfg)
		} else if lowerCaseKey == "email" {
			pvdr = email.NewEmail(v, appCfg)
		} else if lowerCaseKey == "rocketchat" {
			pvdr = rocketchat.NewRocketChat(v, appCfg)
		} else if lowerCaseKey == "mattermost" {
			pvdr = mattermost.NewMattermost(v, appCfg)
		} else if lowerCaseKey == "opsgenie" {
			pvdr = opsgenie.NewOpsgenie(v, appCfg)
		} else if lowerCaseKey == "matrix" {
			pvdr = matrix.NewMatrix(v, appCfg)
		} else if lowerCaseKey == "dingtalk" {
			pvdr = dingtalk.NewDingTalk(v, appCfg)
		} else if lowerCaseKey == "feishu" {
			pvdr = feishu.NewFeiShu(v, appCfg)
		} else if lowerCaseKey == "webhook" {
			pvdr = webhook.NewWebhook(v, appCfg)
		} else if lowerCaseKey == "zenduty" {
			pvdr = zenduty.NewZenduty(v, appCfg)
		} else if lowerCaseKey == "googlechat" {
			pvdr = googlechat.NewGoogleChat(v, appCfg)
		}

		if !reflect.ValueOf(pvdr).IsNil() {
			a.providers = append(a.providers, pvdr)
		}
	}
}

// Notify sends string msg to all providers
func (a *AlertManager) Notify(msg string) {
	klog.InfoS("sending message", "msg", msg)

	for _, prv := range a.providers {
		if err := prv.SendMessage(msg); err != nil {
			klog.ErrorS(err,
				"failed to send msg",
				"provider", prv.Name())
		}
	}
}

// NotifyEvent sends event to all providers
func (a *AlertManager) NotifyEvent(event event.Event) {
	klog.InfoS("sending event", "event", event)

	for _, prv := range a.providers {
		if err := prv.SendEvent(&event); err != nil {
			klog.ErrorS(err,
				"failed to send event",
				"provider", prv.Name())
		}
	}
}
