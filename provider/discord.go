package provider

import (
	"errors"
	"strings"

	"github.com/abahmed/kwatch/event"
	discordgo "github.com/bwmarrin/discordgo"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type discord struct {
	webhook string
}

// NewDiscord returns new Discord object
func NewDiscord(url string) Provider {
	if len(url) == 0 {
		logrus.Warnf("initializing discord with empty webhook url")
	} else {
		logrus.Infof("initializing discord with webhook url: %s", url)
	}

	return &discord{
		webhook: url,
	}
}

// Name returns name of the provider
func (s *discord) Name() string {
	return "Discord"
}

// SendEvent sends event to the provider
func (s *discord) SendEvent(ev *event.Event) error {
	logrus.Debugf("sending to discord event: %v", ev)

	// check config
	if len(s.webhook) == 0 {
		return errors.New("webhook url is empty")
	}

	discordClient, _ := discordgo.New("")

	webhookList := strings.Split(s.webhook, "/")
	webhookToken := webhookList[len(webhookList)-1]
	webhookID := webhookList[len(webhookList)-2]

	// initialize fields with basic info
	fields := []*discordgo.MessageEmbedField{
		{
			Name:   "Name",
			Value:  ev.Name,
			Inline: true,
		},
		{
			Name:   "Container",
			Value:  ev.Container,
			Inline: true,
		},
		{
			Name:   "Namespace",
			Value:  ev.Namespace,
			Inline: true,
		},
		{
			Name:   "Reason",
			Value:  ev.Reason,
			Inline: true,
		},
	}

	// add events part if it exists
	events := strings.TrimSpace(ev.Events)
	if len(events) > 0 {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:  ":mag: Events",
			Value: "```\n" + events + "```",
		})
	}

	// add logs part if it exists
	logs := strings.TrimSpace(ev.Logs)
	if len(logs) > 0 {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:  ":memo: Logs",
			Value: "```\n" + logs + "```",
		})
	}

	// use custom title if it's provided, otherwise use default
	title := viper.GetString("alert.discord.title")
	if len(title) == 0 {
		title = defaultTitle
	}

	// use custom text if it's provided, otherwise use default
	text := viper.GetString("alert.discord.text")
	if len(text) == 0 {
		text = defaultText
	}

	// send message
	_, err := discordClient.WebhookExecute(webhookID, webhookToken, false, &discordgo.WebhookParams{
		Embeds: []*discordgo.MessageEmbed{
			{
				Color:       13041664,
				Title:       title,
				Description: text,
				Fields:      fields,
				Footer: &discordgo.MessageEmbedFooter{
					Text: footer,
				},
			},
		},
	})
	return err
}

// SendMessage sends text message to the provider
func (s *discord) SendMessage(msg string) error {
	// check config
	if len(s.webhook) == 0 {
		return errors.New("webhook url is empty")
	}

	discordClient, _ := discordgo.New("")

	webhookList := strings.Split(s.webhook, "/")
	webhookToken := webhookList[len(webhookList)-1]
	webhookID := webhookList[len(webhookList)-2]

	// send message
	_, err := discordClient.WebhookExecute(webhookID, webhookToken, false, &discordgo.WebhookParams{
		Content: msg,
	})
	return err
}
