package slack

import (
	"fmt"
	"strings"

	"github.com/abahmed/kwatch/event"

	"github.com/sirupsen/logrus"
	slackClient "github.com/slack-go/slack"
)

const (
	footer       = "<https://github.com/abahmed/kwatch|kwatch>"
	defaultTitle = ":red_circle: kwatch detected a crash in pod"
	defaultText  = "There is an issue with container in a pod!"
	chunkSize    = 80
)

type Slack struct {
	webhook string
	title   string
	text    string
	send    func(url string, msg *slackClient.WebhookMessage) error
}

// NewSlack returns new Slack instance
func NewSlack(config map[string]string) *Slack {
	webhook, ok := config["webhook"]
	if !ok || len(webhook) == 0 {
		logrus.Warnf("initializing slack with empty webhook url")
		return nil
	}

	logrus.Infof("initializing slack with webhook url: %s", webhook)

	return &Slack{
		webhook: webhook,
		send:    slackClient.PostWebhook,
	}
}

// Name returns name of the provider
func (s *Slack) Name() string {
	return "Slack"
}

// SendEvent sends event to the provider
func (s *Slack) SendEvent(ev *event.Event) error {
	logrus.Debugf("sending to slack event: %v", ev)

	// use custom title if it's provided, otherwise use default
	title := s.title
	if len(title) == 0 {
		title = defaultTitle
	}

	// use custom text if it's provided, otherwise use default
	text := s.text
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

		for _, chunk := range chunks(events, chunkSize) {
			blocks = append(blocks,
				markdownSectionF("```%s```", chunk))
		}
	}

	// add logs part if it exists
	logs := strings.TrimSpace(ev.Logs)
	if len(logs) > 0 {
		blocks = append(blocks,
			markdownSection(":memo: *Logs*"))

		for _, chunk := range chunks(logs, chunkSize) {
			blocks = append(blocks,
				markdownSectionF("```%s```", chunk))
		}
	}

	msg := slackClient.WebhookMessage{
		Blocks: &slackClient.Blocks{
			BlockSet: append(blocks, markdownSection(footer)),
		},
	}

	// send message
	return s.send(s.webhook, &msg)
}

// SendMessage sends text message to the provider
func (s *Slack) SendMessage(msg string) error {
	sMsg := slackClient.WebhookMessage{
		Text: msg,
	}
	return s.send(s.webhook, &sMsg)
}

func chunks(s string, chunkSize int) []string {
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

func plainSection(txt string) slackClient.SectionBlock {
	return slackClient.SectionBlock{
		Type: "section",
		Text: slackClient.NewTextBlockObject(
			slackClient.PlainTextType,
			txt,
			true,
			false),
	}
}

func markdownSection(txt string) slackClient.SectionBlock {
	return slackClient.SectionBlock{
		Type: "section",
		Text: slackClient.NewTextBlockObject(
			slackClient.MarkdownType,
			txt,
			false,
			true),
	}
}

func markdownF(format string, a ...interface{}) *slackClient.TextBlockObject {
	return slackClient.NewTextBlockObject(
		slackClient.MarkdownType,
		fmt.Sprintf(format, a...),
		false,
		true)
}

func markdownSectionF(
	format string, a ...interface{}) slackClient.SectionBlock {
	return slackClient.SectionBlock{
		Type: "section",
		Text: markdownF(format, a...),
	}
}
