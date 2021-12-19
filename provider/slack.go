package provider

import (
	"errors"
	"strings"

	"github.com/abahmed/kwatch/event"
	"github.com/sirupsen/logrus"
	slackClient "github.com/slack-go/slack"
	"github.com/spf13/viper"
)

type slack struct {
	webhook string
}

// NewSlack returns new Slack object
func NewSlack(url string) Provider {
	if len(url) == 0 {
		logrus.Warnf("initializing slack with empty webhook url")
	} else {
		logrus.Infof("initializing slack with webhook url: %s", url)
	}

	return &slack{
		webhook: url,
	}
}

// Name returns name of the provider
func (s *slack) Name() string {
	return "Slack"
}

// SendEvent sends event to the provider
func (s *slack) SendEvent(ev *event.Event) error {
	logrus.Debugf("sending to slack event: %v", ev)

	// check config
	if len(s.webhook) == 0 {
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
	return slackClient.PostWebhook(s.webhook, &msg)
}

// SendMessage sends text message to the provider
func (s *slack) SendMessage(msg string) error {
	// check config
	if len(s.webhook) == 0 {
		return errors.New("webhook url is empty")
	}

	sMsg := slackClient.WebhookMessage{
		Text: msg,
	}
	return slackClient.PostWebhook(s.webhook, &sMsg)
}
