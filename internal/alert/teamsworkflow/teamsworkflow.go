package teamsworkflow

import (
	"context"
	"encoding/json"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

type adaptiveTextBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
	Wrap bool   `json:"wrap"`
}

type adaptiveCardContent struct {
	Type    string              `json:"type"`
	Version string              `json:"version"`
	Body    []adaptiveTextBlock `json:"body"`
}

type adaptiveCardAttachment struct {
	ContentType string              `json:"contentType"`
	Content     adaptiveCardContent `json:"content"`
}

type teamsPayload struct {
	Type        string                   `json:"type"`
	Attachments []adaptiveCardAttachment `json:"attachments"`
}

type TeamsWorkflow struct {
	sender  transport.Sender
	webhook string

	clusterName string
}

// NewTeamsWorkflow returns a new TeamsWorkflow object

func NewTeamsWorkflow(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *TeamsWorkflow {
	webhook, ok := config["webhook"].(string)
	if !ok || len(webhook) == 0 {
		klog.InfoS("initializing teams workflow with empty webhook url")
		return nil
	}

	if !transport.ValidEndpoint(webhook) {
		klog.InfoS("initializing teamsworkflow with an invalid webhook",
			"setting", "webhook")
		return nil
	}

	klog.InfoS("initializing Teams Workflow with webhook url")

	return &TeamsWorkflow{
		sender:      transport.NewSender(dependencies),
		webhook:     webhook,
		clusterName: clusterName,
	}
}

// Name returns name of the provider
func (t *TeamsWorkflow) Name() string {
	return "Teams Workflow"
}

// SendIncident posts the incident narrative as one card text block, with
// the workload's last output as a Markdown code block.
func (t *TeamsWorkflow) SendIncident(
	ctx context.Context, m notification.Message,
) error {
	text := m.NoteText()
	if len(m.Output) > 0 {
		text += "\n\n```\n" + strings.Join(m.Output, "\n") + "\n```"
	}
	return t.SendMessage(ctx, notification.NeutralizeMentions(text))
}

// SendMessage sends text message to the provider
func (t *TeamsWorkflow) SendMessage(ctx context.Context, msg string) error {
	payload := teamsPayload{
		Type: "message",
		Attachments: []adaptiveCardAttachment{
			{
				ContentType: "application/vnd.microsoft.card.adaptive",
				Content: adaptiveCardContent{
					Type:    "AdaptiveCard",
					Version: "1.4",
					Body: []adaptiveTextBlock{
						{Type: "TextBlock", Text: msg, Wrap: true},
					},
				},
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = t.sender.Send(ctx, transport.Request{
		Provider: t.Name(), URL: t.webhook, Body: body,
		ContentType: "application/json",
	})
	return err
}
