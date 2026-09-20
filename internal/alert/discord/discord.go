package discord

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/delivery/transport"
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
	maxFields = 25
)

type Discord struct {
	httpClient *http.Client
	sender     transport.Sender
	id         string
	token      string
	title      string
	text       string
	send       func(webhookID,
		token string,
		wait bool,
		data *discordgo.WebhookParams,
		options ...discordgo.RequestOption) (st *discordgo.Message, err error)

	// reference for general app configuration
	clusterName string
	clockSource clock.Clock
}

// NewDiscord returns new Discord instance
func NewDiscord(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Discord {
	httpClient := dependencies.HTTPClient
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

	discordClient, err := discordgo.New("")
	if err != nil {
		klog.ErrorS(err, "initializing discord client")
		return nil
	}
	discordClient.Client = httpClient

	title, _ := config["title"].(string)
	text, _ := config["text"].(string)

	return &Discord{
		httpClient:  httpClient,
		sender:      transport.NewSender(dependencies),
		id:          webhookID,
		token:       webhookToken,
		title:       title,
		text:        text,
		send:        discordClient.WebhookExecute,
		clusterName: clusterName,
		clockSource: clock.Require(dependencies.Clock),
	}
}

// Name returns name of the provider
func (d *Discord) Name() string {
	return "Discord"
}

// Verify checks webhook credentials by issuing a GET to the webhook URL.
func (d *Discord) Verify(ctx context.Context) error {
	url := fmt.Sprintf("https://discord.com/api/webhooks/%s/%s", d.id, d.token)
	_, err := d.sender.Send(ctx, transport.Request{
		Provider: "Discord",
		Method:   http.MethodGet,
		URL:      url,
	})
	return err
}

// SendEvent sends an event using the caller's cancellation context.
func (d *Discord) SendEvent(
	ctx context.Context,
	ev *event.Event,
) error {
	klog.V(4).InfoS(
		"sending to discord event",
		"namespace", ev.Namespace,
		"name", ev.PodName,
		"reason", ev.Reason,
		"action", ev.Action,
	)

	// initialize fields with basic info
	fields := []*discordgo.MessageEmbedField{}
	if d.clusterName != "" {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name: "Cluster", Value: d.clusterName, Inline: true,
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
			parts := message.Chunks(events, chunkSize)
			for i, chunk := range parts {
				if len(fields) >= maxFields {
					appendDiscordTruncation(&fields, len(parts)-i)
					break
				}
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

			parts := message.Chunks(logData, chunkSize)
			for i, chunk := range parts {
				if len(fields) >= maxFields {
					appendDiscordTruncation(&fields, len(parts)-i)
					break
				}
				name := ":memo: Logs"
				if len(parts) > 1 {
					name = fmt.Sprintf(
						":memo: Logs (%d/%d)",
						i+1,
						len(parts),
					)
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
		},
		discordgo.WithContext(ctx),
	)
	return wrapDiscordRateLimit(err)
}

func appendDiscordTruncation(
	fields *[]*discordgo.MessageEmbedField,
	remaining int,
) {
	if len(*fields) >= maxFields {
		return
	}
	*fields = append(*fields, &discordgo.MessageEmbedField{
		Name:  ":warning: Evidence truncated",
		Value: fmt.Sprintf("… (truncated, %d more chunk(s))", remaining),
	})
}

// SendMessage sends text using the caller's cancellation context.
func (d *Discord) SendMessage(
	ctx context.Context,
	msg string,
) error {
	// send message
	_, err := d.send(
		d.id,
		d.token,
		false,
		&discordgo.WebhookParams{
			Content: msg,
		},
		discordgo.WithContext(ctx),
	)
	return wrapDiscordRateLimit(err)
}

// SendIncident implements delivery.ThreadProvider.
// It renders the incident using the Report model and DiscordRenderer,
// producing a rich embed with context-adaptive fields.
func (d *Discord) SendIncident(
	ctx context.Context,
	inc *model.Incident,
	action model.IncidentAction,
) error {
	return d.SendIncidentWithInsight(ctx, inc, action, nil)
}

// SendIncidentWithInsight implements delivery.InsightThreadProvider, so the
// diagnosis — likely cause, impact, recent changes — is rendered rather than
// dropped on the way to this provider.
func (d *Discord) SendIncidentWithInsight(
	ctx context.Context,
	inc *model.Incident,
	action model.IncidentAction,
	ins *insight.Insight,
) error {
	text := message.RenderIncidentWithInsight(
		inc,
		action,
		ins,
		message.NewDiscordRenderer(),
		d.clusterName,
		d.clockSource,
	)
	if text == "" {
		return nil
	}
	return d.SendMessage(ctx, text)
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
	var restErr *discordgo.RESTError
	if errors.As(err, &restErr) && restErr.Response != nil {
		return transport.ClassifyHTTPStatus(restErr.Response.StatusCode, err)
	}
	return err
}
