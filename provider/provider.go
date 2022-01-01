package provider

import (
	"github.com/abahmed/kwatch/event"
	"github.com/spf13/viper"
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

// New returns a new Provider object
func New(name string) Provider {
	switch name {
	case "slack":
		return NewSlack(viper.GetString("alert.slack.webhook"))
	case "discord":
		return NewDiscord(viper.GetString("alert.discord.webhook"))
	case "pagerduty":
		return NewPagerDuty(viper.GetString("alert.pagerduty.integrationKey"))
	case "telegram":
		return NewTelegram(viper.GetString("alert.telegram.token"), viper.GetString("alert.telegram.chatId"))
	case "teams":
		return NewTeams(viper.GetString("alert.teams.webhook"))
	default:
		return nil
	}
}
