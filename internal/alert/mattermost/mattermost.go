package mattermost

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/safetext"
	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

type Mattermost struct {
	sender  transport.Sender
	webhook string

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
	Fields []mmField `json:"fields"`
}

type mmPayload struct {
	Text        string         `json:"text"`
	Attachments []mmAttachment `json:"attachments,omitempty"`
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

	if !transport.ValidEndpoint(webhook) {
		klog.InfoS("initializing mattermost with an invalid webhook",
			"setting", "webhook")
		return nil
	}

	klog.InfoS("initializing mattermost with webhook configured")

	return &Mattermost{
		sender:      transport.NewSender(dependencies),
		webhook:     webhook,
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

	b, err := m.buildMessage(msg)
	if err != nil {
		return err
	}
	return m.sendAPI(ctx, b)
}

// SendIncident posts the incident narrative as the message text, with the
// workload's last output as a code block. The cluster rides in an
// attachment field so the text starts with the status marker.
func (m *Mattermost) SendIncident(
	ctx context.Context, msg notification.Message,
) error {
	klog.V(4).InfoS("sending incident to mattermost",
		"component", "mattermost", "conversation", msg.Key)
	b, err := m.buildIncident(msg)
	if err != nil {
		return err
	}
	return m.sendAPI(ctx, b)
}

func (m *Mattermost) sendAPI(ctx context.Context, content []byte) error {
	_, err := m.sender.Send(ctx, transport.Request{
		Provider: "Mattermost", URL: m.webhook, Body: content,
	})
	return err
}

func (m *Mattermost) buildMessage(msg string) ([]byte, error) {
	return marshalPayload(mmPayload{
		Text: notification.NeutralizeMentions(msg),
	})
}

func (m *Mattermost) buildIncident(msg notification.Message) ([]byte, error) {
	text := safetext.RichWithOutput(msg,
		notification.MarkdownDialect(nil, "\n"), "\n", 0)
	payload := mmPayload{Text: text}
	if m.clusterName != "" {
		payload.Attachments = []mmAttachment{{Fields: []mmField{{
			Title: "Cluster", Value: m.clusterName, Short: true,
		}}}}
	}
	return marshalPayload(payload)
}

func marshalPayload(payload mmPayload) ([]byte, error) {
	str, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal mattermost payload: %w", err)
	}
	return str, nil
}
