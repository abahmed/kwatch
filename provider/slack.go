package provider

import (
	"errors"
	"strings"

	"github.com/abahmed/kwatch/event"
	"github.com/sirupsen/logrus"
	slackClient "github.com/slack-go/slack"
	"github.com/spf13/viper"
)

const (
	footer       = "<https://github.com/abahmed/kwatch|kwatch>"
	defaultTitle = ":red_circle: kwatch detected crash in a pod"
	defaultText  = "There is an issue with container in a pod!"
)

type slack struct{}

// NewSlack returns new Slack object
func NewSlack() Provider {
	return &slack{}
}

// Name returns name of the provider
func (s *slack) Name() string {
	return "Slack"
}

// SendEvent sends event to the provider
func (s *slack) SendEvent(ev *event.Event) error {
	logrus.Debugf("sending to slack event: %v", ev)

	// check config
	url := viper.GetString("providers.slack.webhook")
	if len(url) == 0 {
		return errors.New("webhook url is empty")
	}

	// initialize fields with basic info
	fields := []slackClient.AttachmentField{
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
		fields = append(fields, slackClient.AttachmentField{
			Title: ":mag: Events",
			Value: "```\n" + events + "```",
		})
	}

	// add logs part if it exists
	logs := strings.TrimSpace(ev.Logs)
	if len(logs) > 0 {
		fields = append(fields, slackClient.AttachmentField{
			Title: ":memo: Logs",
			Value: "```\n" + logs + "```",
		})
	}

	// use custom title if it's provided, otherwise use default
	title := viper.GetString("providers.slack.title")
	if len(title) == 0 {
		title = defaultTitle
	}

	// use custom text if it's provided, otherwise use default
	text := viper.GetString("providers.slack.text")
	if len(text) == 0 {
		text = defaultText
	}

	msg := slackClient.WebhookMessage{
		Attachments: []slackClient.Attachment{
			{
				Color:      "danger",
				Title:      title,
				Text:       text,
				Fields:     fields,
				MarkdownIn: []string{"fields"},
				Footer:     footer,
			},
		},
	}

	// send message
	return slackClient.PostWebhook(url, &msg)
}

// SendMessage sends text message to the provider
func (s *slack) SendMessage(msg string) error {
	// check config
	url := viper.GetString("providers.slack.webhook")
	if len(url) == 0 {
		return errors.New("webhook url is empty")
	}

	sMsg := slackClient.WebhookMessage{
		Text: msg,
	}
	return slackClient.PostWebhook(url, &sMsg)
}
