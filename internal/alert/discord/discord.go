package discord

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/message"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/ratelimit"

	discordgo "github.com/bwmarrin/discordgo"
	"k8s.io/klog/v2"
)

const (
	chunkSize = 1024
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
		klog.InfoS("initializing discord with empty webhook url")
		return nil
	}

	webhookList := strings.Split(webhook, "/")
	if len(webhookList) <= 1 {
		klog.InfoS("initializing discord with missing id or token")
		return nil
	}
	klog.InfoS("initializing discord with webhook configured")

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
func (d *Discord) Name() string {
	return "Discord"
}

// Verify checks webhook credentials by issuing a GET to the webhook URL.
func (d *Discord) Verify() error {
	url := fmt.Sprintf("https://discord.com/api/webhooks/%s/%s", d.id, d.token)
	_, err := util.Send(util.Request{
		Provider: "Discord",
		Method:   http.MethodGet,
		URL:      url,
	})
	return err
}

// SendEvent sends event to the provider
func (d *Discord) SendEvent(ev *event.Event) error {
	klog.V(4).InfoS("sending to discord event", "event", ev)

	// initialize fields with basic info
	fields := []*discordgo.MessageEmbedField{}
	if d.appCfg.ClusterName != "" {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name: "Cluster", Value: d.appCfg.ClusterName, Inline: true,
		})
	}
	if ev.PodName != "" {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name: "Name", Value: ev.PodName, Inline: true,
		})
	}
	if ev.ContainerName != "" {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name: "Container", Value: ev.ContainerName, Inline: true,
		})
	}
	if ev.Namespace != "" {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name: "Namespace", Value: ev.Namespace, Inline: true,
		})
	}
	if ev.NodeName != "" {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name: "Node", Value: ev.NodeName, Inline: true,
		})
	}
	if ev.Reason != "" {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name: "Reason", Value: ev.Reason, Inline: true,
		})
	}

	// add events part if it exists
	if ev.IncludeEvents {
		events := strings.TrimSpace(ev.Events)
		if len(events) > 0 {
			for _, chunk := range util.Chunks(events, chunkSize) {
				fields = append(fields, &discordgo.MessageEmbedField{
					Name:  ":mag: Events",
					Value: "```\n" + chunk + "```",
				})
			}
		}
	}

	// add logs part if it exists
	if ev.IncludeLogs {
		logs := strings.TrimSpace(ev.Logs)
		if len(logs) > 0 {
			logData := logs

			const maxFields = 25
			var totalFields int
			parts := util.Chunks(logData, chunkSize)
			for _, chunk := range parts {
				name := ":memo: Logs"
				totalFields++
				if len(parts) > 1 {
					name = fmt.Sprintf(
						":memo: Logs (%d/%d)",
						totalFields,
						len(parts),
					)
				}
				if totalFields > maxFields {
					remaining := len(parts) - (totalFields - 1)
					fields = append(fields, &discordgo.MessageEmbedField{
						Name: ":memo: Logs",
						Value: fmt.Sprintf(
							"… (truncated, %d more chunk(s))",
							remaining,
						),
					})
					break
				}
				fields = append(fields, &discordgo.MessageEmbedField{
					Name:  name,
					Value: "```\n" + chunk + "```",
				})
			}
		}
	}

	// use custom title if it's provided, otherwise use default
	title := d.title
	if len(title) == 0 {
		title = constant.DefaultTitle
	}

	// use custom text if it's provided, otherwise use default
	text := d.text
	if len(text) == 0 {
		text = constant.DefaultText
	}

	// send message
	_, err := d.send(
		d.id,
		d.token,
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
	return wrapDiscordRateLimit(err)
}

// SendMessage sends text message to the provider
func (d *Discord) SendMessage(msg string) error {
	// send message
	_, err := d.send(
		d.id,
		d.token,
		false,
		&discordgo.WebhookParams{
			Content: msg,
		})
	return wrapDiscordRateLimit(err)
}

// SendIncident implements alert.ThreadProvider.
// It renders the incident using the Report model and DiscordRenderer,
// producing a rich embed with context-adaptive fields.
func (d *Discord) SendIncident(
	inc *model.Incident,
	action model.IncidentAction,
) error {
	return d.SendIncidentWithInsight(inc, action, nil)
}

// SendIncidentWithInsight implements alert.InsightThreadProvider, so the
// diagnosis — likely cause, impact, recent changes — is rendered rather than
// dropped on the way to this provider.
func (d *Discord) SendIncidentWithInsight(
	inc *model.Incident,
	action model.IncidentAction,
	ins *insight.Insight,
) error {
	text := util.RenderIncidentWithInsight(
		inc,
		action,
		ins,
		message.NewDiscordRenderer(),
		d.appCfg.ClusterName,
	)
	if text == "" {
		return nil
	}
	return d.SendMessage(text)
}

func wrapDiscordRateLimit(err error) error {
	if err == nil {
		return nil
	}
	var rle *discordgo.RateLimitError
	if errors.As(err, &rle) {
		return &ratelimit.Error{
			Provider:   "Discord",
			StatusCode: http.StatusTooManyRequests,
			RetryAfter: rle.RetryAfter,
		}
	}
	return err
}
