package googlechat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

type GoogleChat struct {
	sender  transport.Sender
	webhook string

	// reference for general app configuration
	clusterName string
	clockSource clock.Clock
}

type payload struct {
	Text string `json:"text"`
}

// NewGoogleChat returns new google chat instance

func NewGoogleChat(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *GoogleChat {
	webhook, ok := config["webhook"].(string)
	if !ok || len(webhook) == 0 {
		klog.InfoS("initializing Google Chat with empty webhook url")
		return nil
	}

	if !transport.ValidEndpoint(webhook) {
		klog.InfoS("initializing googlechat with an invalid webhook",
			"setting", "webhook")
		return nil
	}

	klog.InfoS("initializing Google Chat with webhook configured")

	return &GoogleChat{
		sender:      transport.NewSender(dependencies),
		webhook:     webhook,
		clusterName: clusterName,
		clockSource: clock.Require(dependencies.Clock),
	}
}

// Name returns name of the provider
func (g *GoogleChat) Name() string {
	return "Google Chat"
}

// SendIncident posts the incident narrative, followed by the workload's
// last output as a code block when there is one.
func (g *GoogleChat) SendIncident(
	ctx context.Context, m notification.Message,
) error {
	b, err := g.buildRequestBody(incidentText(m))
	if err != nil {
		return err
	}
	return g.sendAPI(ctx, b)
}

func (g *GoogleChat) sendAPI(ctx context.Context, reqBody []byte) error {
	_, err := g.sender.Send(ctx, transport.Request{
		Provider: "GoogleChat", URL: g.webhook, Body: reqBody,
	})
	return err
}

// SendMessage sends text message to the provider
func (g *GoogleChat) SendMessage(ctx context.Context, msg string) error {
	b, err := g.buildRequestBody(msg)
	if err != nil {
		return err
	}
	return g.sendAPI(ctx, b)
}

func (g *GoogleChat) buildRequestBody(text string) ([]byte, error) {
	msgPayload := &payload{
		Text: text,
	}

	jsonBytes, err := json.Marshal(msgPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal google chat payload: %w", err)
	}
	return jsonBytes, nil
}

// incidentText is the Note with the last output as a code block. Mentions
// are neutralized so log text cannot notify a whole space.
func incidentText(m notification.Message) string {
	text := m.NoteText()
	if len(m.Output) > 0 {
		text += "\n```\n" + strings.Join(m.Output, "\n") + "\n```"
	}
	return notification.NeutralizeGoogleChatMentions(text)
}
