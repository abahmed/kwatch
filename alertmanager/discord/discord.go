package discord

import (
	"strings"

	"github.com/abahmed/kwatch/config"
	"github.com/abahmed/kwatch/constant"
	"github.com/abahmed/kwatch/event"

	discordgo "github.com/bwmarrin/discordgo"
	"github.com/sirupsen/logrus"
)

type Discord struct {
	id    string
	token string
	title string
	text  string
	send  func(webhookID,
		token string,
		wait bool,
		data *discordgo.WebhookParams,
		options ...discordgo.RequestOption) (st *discordgo.Message, err error)

	// reference for general app configuration
	appCfg *config.App
}

// NewDiscord returns new Discord instance
func NewDiscord(config map[string]interface{}, appCfg *config.App) *Discord {
	webhook, ok := config["webhook"].(string)
	if !ok || len(webhook) == 0 {
		logrus.Warnf("initializing discord with empty webhook url")
		return nil
	}

	webhookList := strings.Split(webhook, "/")
	if len(webhookList) <= 1 {
		logrus.Warnf("initializing discord with missing id or token")
		return nil
	}
	logrus.Infof("initializing discord with webhook url: %s", webhook)

	webhookToken := webhookList[len(webhookList)-1]
	webhookID := webhookList[len(webhookList)-2]

	discordClient, _ := discordgo.New("")

	title, _ := config["title"].(string)
	text, _ := config["text"].(string)

	return &Discord{
		id:     webhookID,
		token:  webhookToken,
		title:  title,
		text:   text,
		send:   discordClient.WebhookExecute,
		appCfg: appCfg,
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
			Name:   "Cluster",
			Value:  s.appCfg.ClusterName,
			Inline: true,
		},
		{
			Name:   "Name",
			Value:  ev.PodName,
			Inline: true,
		},
		{
			Name:   "Container",
			Value:  ev.ContainerName,
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
	title := s.title
	if len(title) == 0 {
		title = constant.DefaultTitle
	}

	// use custom text if it's provided, otherwise use default
	text := s.text
	if len(text) == 0 {
		text = constant.DefaultText
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
						Text: constant.Footer,
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
