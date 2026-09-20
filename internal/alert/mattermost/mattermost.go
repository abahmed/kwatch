package mattermost

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/message"
	"github.com/abahmed/kwatch/internal/model"
)

type Mattermost struct {
	sender  transport.Sender
	webhook string
	title   string
	text    string

	// reference for general app configuration
	clusterName string
	clockSource clock.Clock
}

type mmField struct {
	Short bool        `json:"short"`
	Title string      `json:"title"`
	Value interface{} `json:"value"`
}

type mmAttachment struct {
	Title  string    `json:"title"`
	Text   string    `json:"text"`
	Fields []mmField `json:"fields"`
}

type mmPayload struct {
	Text        string         `json:"text"`
	Attachments []mmAttachment `json:"attachments"`
}

// NewMattermost returns new mattermost instance

func NewMattermost(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Mattermost {
	webhook, ok := config["webhook"].(string)
	if !ok || len(webhook) == 0 {
		klog.InfoS("initializing mattermost with empty webhook url")
		return nil
	}

	klog.InfoS("initializing mattermost with webhook configured")

	title, _ := config["title"].(string)
	text, _ := config["text"].(string)

	return &Mattermost{
		sender:      transport.NewSender(dependencies),
		webhook:     webhook,
		title:       title,
		text:        text,
		clusterName: clusterName,
		clockSource: clock.Require(dependencies.Clock),
	}
}

// Name returns name of the provider
func (m *Mattermost) Name() string {
	return "Mattermost"
}

// SendMessage sends text message to the provider
func (m *Mattermost) SendMessage(ctx context.Context, msg string) error {
	klog.V(4).InfoS(
		"sending message to mattermost",
		"messageLength", len(msg),
	)

	b, err := m.buildMessage(nil, &msg)
	if err != nil {
		return err
	}
	return m.sendAPI(ctx, b)
}

// SendEvent sends event to the provider
func (m *Mattermost) SendEvent(ctx context.Context, e *event.Event) error {
	klog.V(4).InfoS(
		"sending to mattermost event",
		"namespace", e.Namespace,
		"name", e.PodName,
		"reason", e.Reason,
		"action", e.Action,
	)

	b, err := m.buildMessage(e, nil)
	if err != nil {
		return err
	}
	return m.sendAPI(ctx, b)
}

// SendIncident implements delivery.ThreadProvider.
// It renders the incident using the Report model and PlaintextRenderer,
// producing a context-adaptive text message.
func (m *Mattermost) SendIncident(
	ctx context.Context,
	inc *model.Incident,
	action model.IncidentAction,
) error {
	return m.SendIncidentWithInsight(ctx, inc, action, nil)
}

// SendIncidentWithInsight implements delivery.InsightThreadProvider, so the
// diagnosis — likely cause, impact, recent changes — is rendered rather than
// dropped on the way to this provider.
func (m *Mattermost) SendIncidentWithInsight(
	ctx context.Context,
	inc *model.Incident,
	action model.IncidentAction,
	ins *insight.Insight,
) error {
	text := message.RenderIncidentWithInsight(
		inc,
		action,
		ins,
		message.NewPlainTextRenderer(),
		m.clusterName,
		m.clockSource,
	)
	if text == "" {
		return nil
	}
	return m.SendMessage(ctx, text)
}

func (m *Mattermost) sendAPI(ctx context.Context, content []byte) error {
	_, err := m.sender.Send(ctx, transport.Request{
		Provider: "Mattermost", URL: m.webhook, Body: content,
	})
	return err
}

func (m *Mattermost) buildMessage(e *event.Event, msg *string) ([]byte, error) {
	payload := mmPayload{}

	if msg != nil && len(*msg) > 0 {
		payload.Text = *msg
	}

	if e != nil {
		logs := strings.TrimSpace(e.Logs)
		events := strings.TrimSpace(e.Events)

		// use custom title if it's provided, otherwise use default
		title := m.title
		if len(title) == 0 {
			title = constant.DefaultTitle
		}

		// use custom text if it's provided, otherwise use default
		text := m.text
		if len(text) == 0 {
			text = constant.DefaultText
		}

		mmFields := []mmField{}
		if m.clusterName != "" {
			mmFields = append(
				mmFields,
				mmField{
					Title: "Cluster",
					Value: m.clusterName,
					Short: true,
				},
			)
		}
		if e.PodName != "" {
			mmFields = append(
				mmFields,
				mmField{Title: "Name", Value: e.PodName, Short: true},
			)
		}
		if e.ContainerName != "" {
			mmFields = append(
				mmFields,
				mmField{
					Title: "Container",
					Value: e.ContainerName,
					Short: true,
				},
			)
		}
		if e.Namespace != "" {
			mmFields = append(
				mmFields,
				mmField{Title: "Namespace", Value: e.Namespace, Short: true},
			)
		}
		if e.NodeName != "" {
			mmFields = append(
				mmFields,
				mmField{Title: "Node", Value: e.NodeName, Short: true},
			)
		}
		if e.Reason != "" {
			mmFields = append(
				mmFields,
				mmField{Title: "Reason", Value: e.Reason, Short: true},
			)
		}
		if logs != "" {
			mmFields = append(
				mmFields,
				mmField{
					Title: ":memo: Logs",
					Value: "```\n" + logs + "\n```",
					Short: false,
				},
			)
		}
		if events != "" {
			mmFields = append(
				mmFields,
				mmField{
					Title: ":mag: Events",
					Value: "```\n" + events + " \n```",
					Short: false,
				},
			)
		}

		payload.Attachments = []mmAttachment{
			{
				Title:  title,
				Text:   text,
				Fields: mmFields,
			},
		}
	}

	str, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal mattermost payload: %w", err)
	}
	return str, nil
}
