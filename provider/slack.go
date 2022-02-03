package provider

import (
	"errors"
	"fmt"
	"strings"

	"github.com/abahmed/kwatch/event"
	"github.com/sirupsen/logrus"
	slackClient "github.com/slack-go/slack"
	"github.com/spf13/viper"
)

type slack struct {
	webhook string
}

// NewSlack returns new Slack instance
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

	// use custom title if it's provided, otherwise use default
	title := viper.GetString("alert.slack.title")
	if len(title) == 0 {
		title = defaultTitle
	}

	// use custom text if it's provided, otherwise use default
	text := viper.GetString("alert.slack.text")
	if len(text) == 0 {
		text = defaultText
	}

	blocks := []slackClient.Block{
		markdownSection(title),
		plainSection(text),
		slackClient.SectionBlock{
			Type: "section",
			Fields: []*slackClient.TextBlockObject{
				markdownF("*Name*\n%s", ev.Name),
				markdownF("*Container*\n%s", ev.Container),
				markdownF("*Namespace*\n%s", ev.Namespace),
				markdownF("*Reason*\n%s", ev.Reason),
			},
		},
	}

	// add events part if it exists
	events := strings.TrimSpace(ev.Events)
	if len(events) > 0 {
		blocks = append(blocks,
			markdownSection(":mag: *Events*"))

		for _, chunk := range chunks(events, 2000) {
			blocks = append(blocks,
				markdownSectionF("```%s```", chunk))
		}
	}

	// add logs part if it exists
	logs := strings.TrimSpace(ev.Logs)
	if len(logs) > 0 {
		blocks = append(blocks,
			markdownSection(":memo: *Logs*"))

		for _, chunk := range chunks(logs, 2000) {
			blocks = append(blocks,
				markdownSectionF("```%s```", chunk))
		}
	}

	msg := slackClient.WebhookMessage{
		Blocks: &slackClient.Blocks{BlockSet: append(blocks, markdownSection(footer))},
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

func chunks(s string, chunkSize int) []string {
	if len(s) == 0 {
		return nil
	}
	if chunkSize >= len(s) {
		return []string{s}
	}
	var chunks []string = make([]string, 0, (len(s)-1)/chunkSize+1)
	currentLen := 0
	currentStart := 0
	for i := range s {
		if currentLen == chunkSize {
			chunks = append(chunks, s[currentStart:i])
			currentLen = 0
			currentStart = i
		}
		currentLen++
	}
	chunks = append(chunks, s[currentStart:])
	return chunks
}

func plain(txt string) *slackClient.TextBlockObject {
	return slackClient.NewTextBlockObject(slackClient.PlainTextType, txt, true, false)
}
func plainSection(txt string) slackClient.SectionBlock {
	return slackClient.SectionBlock{
		Type: "section",
		Text: plain(txt),
	}
}
func markdown(txt string) *slackClient.TextBlockObject {
	return slackClient.NewTextBlockObject(slackClient.MarkdownType, txt, false, true)
}
func markdownSection(txt string) slackClient.SectionBlock {
	return slackClient.SectionBlock{
		Type: "section",
		Text: markdown(txt),
	}
}
func markdownF(format string, a ...interface{}) *slackClient.TextBlockObject {
	return slackClient.NewTextBlockObject(slackClient.MarkdownType, fmt.Sprintf(format, a...), false, true)
}
func markdownSectionF(format string, a ...interface{}) slackClient.SectionBlock {
	return slackClient.SectionBlock{
		Type: "section",
		Text: markdownF(format, a...),
	}
}
