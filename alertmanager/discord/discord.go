package discord

import (
	"strings"

	"github.com/abahmed/kwatch/event"
	discordgo "github.com/bwmarrin/discordgo"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

const (
	footer       = "<https://github.com/abahmed/kwatch|kwatch>"
	defaultTitle = ":red_circle: kwatch detected a crash in pod"
	defaultText  = "There is an issue with container in a pod!"
	chunkSize    = 80
)

type Discord struct {
	id    string
	token string
	send  func(
		webhookID,
		token string,
		wait bool,
		data *discordgo.WebhookParams) (st *discordgo.Message, err error)
}

// NewDiscord returns new Discord instance
func NewDiscord(config map[string]string) *Discord {
	webhook, ok := config["webhook"]
	if !ok || len(webhook) == 0 {
		logrus.Warnf("initializing discord with empty webhook url")
		return nil
	}

	logrus.Infof("initializing discord with webhook url: %s", webhook)

	webhookList := strings.Split(webhook, "/")
	if len(webhookList) <= 1 {
		logrus.Warnf("initializing discord with missing id or token")
		return nil
	}

	webhookToken := webhookList[len(webhookList)-1]
	webhookID := webhookList[len(webhookList)-2]

	discordClient, _ := discordgo.New("")

	return &Discord{
		id:    webhookID,
		token: webhookToken,
		send:  discordClient.WebhookExecute,
	}
}

// Name returns name of the provider
func (s *Discord) Name() string {
	return "Discord"
}

// SendEvent sends event to the provider
func (s *Discord) SendEvent(ev *event.Event) error {
	logrus.Debugf("sending to discord event: %v", ev)

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
		logData := logs

		if len(logData) > 1024 {
			logData = logs[:1024]
		}

		fields = append(fields, &discordgo.MessageEmbedField{
			Name:  ":memo: Logs",
			Value: "```\n" + logData + "```",
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
	_, err := s.send(
		s.id,
		s.token,
		false,
		&discordgo.WebhookParams{
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
func (s *Discord) SendMessage(msg string) error {
	// send message
	_, err := s.send(
		s.id,
		s.token,
		false,
		&discordgo.WebhookParams{
			Content: msg,
		})
	return err
}
