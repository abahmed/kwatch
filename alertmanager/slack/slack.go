package slack

import (
	"context"
	"fmt"
	"strings"

	"github.com/abahmed/kwatch/config"
	"github.com/abahmed/kwatch/constant"
	"github.com/abahmed/kwatch/event"

	"github.com/sirupsen/logrus"
	slackClient "github.com/slack-go/slack"
)

const (
	chunkSize = 2000
)

type Slack struct {
	title   string
	text    string
	channel string
	appCfg  *config.App

	// webhook mode
	webhook string
	send    func(url string, msg *slackClient.WebhookMessage) error

	// token mode
	token     string
	apiClient *slackClient.Client
}

// NewSlack returns new Slack instance
func NewSlack(config map[string]interface{}, appCfg *config.App) *Slack {
	title, _ := config["title"].(string)
	text, _ := config["text"].(string)

	// token mode: requires token + channel
	token, hasToken := config["token"].(string)
	channel, hasChannel := config["channel"].(string)
	if hasToken && len(token) > 0 {
		if !hasChannel || len(channel) == 0 {
			logrus.Warnf("initializing slack with token but missing channel")
			return nil
		}
		logrus.Infof("initializing slack with token and channel: %s", channel)
		return &Slack{
			token:     token,
			channel:   channel,
			title:     title,
			text:      text,
			appCfg:    appCfg,
			apiClient: slackClient.New(token),
		}
	}

	// webhook mode: requires webhook
	webhook, ok := config["webhook"].(string)
	if !ok || len(webhook) == 0 {
		logrus.Warnf("initializing slack with empty webhook url and no token")
		return nil
	}

	logrus.Infof("initializing slack with webhook url: %s", webhook)

	return &Slack{
		webhook: webhook,
		channel: channel,
		title:   title,
		text:    text,
		appCfg:  appCfg,
		send:    slackClient.PostWebhook,
	}
}

// Name returns name of the provider
func (s *Slack) Name() string {
	return "Slack"
}

// SendEvent sends event to the provider
func (s *Slack) SendEvent(ev *event.Event) error {
	logrus.Infof("sending to slack event: %v", ev)

	// use custom title if it's provided, otherwise use default
	title := s.title
	if len(title) == 0 {
		title = constant.DefaultTitle
	}

	// use custom text if it's provided, otherwise use default
	text := s.text
	if len(text) == 0 {
		text = constant.DefaultText
	}

	blocks := []slackClient.Block{
		markdownSection(title),
		plainSection(text),
		slackClient.SectionBlock{
			Type: "section",
			Fields: []*slackClient.TextBlockObject{
				markdownF("*Cluster*\n%s", s.appCfg.ClusterName),
				markdownF("*Name*\n%s", ev.PodName),
				markdownF("*Container*\n%s", ev.ContainerName),
				markdownF("*Namespace*\n%s", ev.Namespace),
				markdownF("*Node*\n%s", ev.NodeName),
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

	// send message
	return s.sendAPI(&slackClient.WebhookMessage{
		Blocks: &slackClient.Blocks{
			BlockSet: append(blocks, markdownSection(constant.Footer)),
		},
	})
}

// SendMessage sends text message to the provider
func (s *Slack) SendMessage(msg string) error {
	return s.sendAPI(&slackClient.WebhookMessage{
		Text: msg,
	})
}

func (s *Slack) sendAPI(msg *slackClient.WebhookMessage) error {
	if s.apiClient != nil {
		return s.sendAPIWithToken(msg)
	}
	if len(s.channel) > 0 {
		msg.Channel = s.channel
	}
	return s.send(s.webhook, msg)
}

func (s *Slack) sendAPIWithToken(msg *slackClient.WebhookMessage) error {
	opts := []slackClient.MsgOption{}
	if len(msg.Text) > 0 {
		opts = append(opts, slackClient.MsgOptionText(msg.Text, false))
	}
	if msg.Blocks != nil {
		opts = append(opts, slackClient.MsgOptionBlocks(msg.Blocks.BlockSet...))
	}
	_, _, err := s.apiClient.PostMessageContext(
		context.Background(),
		s.channel,
		opts...,
	)
	return err
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
