package provider

import (
	"errors"
	"strings"

	"github.com/abahmed/kwatch/event"
	"github.com/sirupsen/logrus"
	"github.com/slack-go/slack"
	"github.com/spf13/viper"
)

const (
	Footer       = "<https://github.com/abahmed/kwatch|kwatch>"
	DefaultTitle = ":red_circle: Pod Crash"
	DefaultText  = "There is an issue with your pod!"
)

type Slack struct{}

// NewSlack returns new Slack object
func NewSlack() Provider {
	return &Slack{}
}

func (s *Slack) Name() string {
	return "Slack"
}

func (s *Slack) SendEvent(ev *event.Event) error {
	logrus.Debugf("sending to slack event: %v", ev)

	// check config
	url := viper.GetString("providers.slack.webhook")
	if len(url) == 0 {
		return errors.New("webhook url is empty")
	}

	// initialize fields with basic info
	fields := []slack.AttachmentField{
		{
			Title: "Name",
			Value: ev.Name,
			Short: true,
		},
		{
			Title: "Container",
			Value: ev.Container,
			Short: true,
		},
		{
			Title: "Namespace",
			Value: ev.Namespace,
			Short: true,
		},
		{
			Title: "Reason",
			Value: ev.Reason,
			Short: true,
		},
	}

	// add events part if it exists
	events := strings.TrimSpace(ev.Events)
	if len(events) > 0 {
		fields = append(fields, slack.AttachmentField{
			Title: ":mag: Events",
			Value: "```\n" + events + "```",
		})
	}

	// add logs part if it exists
	logs := strings.TrimSpace(ev.Logs)
	if len(logs) > 0 {
		fields = append(fields, slack.AttachmentField{
			Title: ":memo: Logs",
			Value: "```\n" + logs + "```",
		})
	}

	// use custom title if it's provided, otherwise use default
	title := viper.GetString("providers.slack.title")
	if len(title) == 0 {
		title = DefaultTitle
	}

	// use custom text if it's provided, otherwise use default
	text := viper.GetString("providers.slack.text")
	if len(text) == 0 {
		text = DefaultText
	}

	msg := slack.WebhookMessage{
		Attachments: []slack.Attachment{
			{
				Color:      "danger",
				Title:      title,
				Text:       text,
				Fields:     fields,
				MarkdownIn: []string{"fields"},
				Footer:     Footer,
			},
		},
	}

	// send message
	return slack.PostWebhook(url, &msg)
}

func (s *Slack) SendMessage(msg string) error {
	// check config
	url := viper.GetString("providers.slack.webhook")
	if len(url) == 0 {
		return errors.New("webhook url is empty")
	}

	sMsg := slack.WebhookMessage{
		Text: msg,
	}
	return slack.PostWebhook(url, &sMsg)
}
