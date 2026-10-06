package rocketchat

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

type RocketChat struct {
	sender  transport.Sender
	webhook string

	// reference for general app configuration
	clusterName string
	clockSource clock.Clock
}

type rocketChatWebhookPayload struct {
	Text string `json:"text"`
}

// NewRocketChat returns new rocket chat instance

func NewRocketChat(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *RocketChat {
	webhook, ok := config["webhook"].(string)
	if !ok || len(webhook) == 0 {
		klog.InfoS("initializing Rocket Chat with empty webhook url")
		return nil
	}

	if !transport.ValidEndpoint(webhook) {
		klog.InfoS("initializing rocketchat with an invalid webhook",
			"setting", "webhook")
		return nil
	}

	klog.InfoS("initializing Rocket Chat with webhook configured")

	return &RocketChat{
		sender:      transport.NewSender(dependencies),
		webhook:     webhook,
		clusterName: clusterName,
		clockSource: clock.Require(dependencies.Clock),
	}
}

// Name returns name of the provider
func (r *RocketChat) Name() string {
	return "Rocket Chat"
}

// SendIncident posts the incident narrative, followed by the workload's
// last output as a code block when there is one.
func (r *RocketChat) SendIncident(
	ctx context.Context, m notification.Message,
) error {
	b, err := r.buildRequestBodyRocketChat(incidentText(m))
	if err != nil {
		return err
	}
	return r.sendByRocketChatApi(ctx, b)
}

func (r *RocketChat) sendByRocketChatApi(
	ctx context.Context,
	reqBody []byte,
) error {
	_, err := r.sender.Send(ctx, transport.Request{
		Provider: "RocketChat", URL: r.webhook, Body: reqBody,
	})
	return err
}

// SendMessage sends text message to the provider
func (r *RocketChat) SendMessage(ctx context.Context, msg string) error {
	b, err := r.buildRequestBodyRocketChat(msg)
	if err != nil {
		return err
	}
	return r.sendByRocketChatApi(ctx, b)
}

func (r *RocketChat) buildRequestBodyRocketChat(text string) ([]byte, error) {
	msgPayload := &rocketChatWebhookPayload{
		Text: notification.NeutralizeMentions(text),
	}

	jsonBytes, err := json.Marshal(msgPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal rocketchat payload: %w", err)
	}
	return jsonBytes, nil
}

// incidentText is the Note with the last output as a Markdown code block.
func incidentText(m notification.Message) string {
	return safetext.RichWithOutput(m,
		notification.MarkdownDialect(nil, "\n"), "\n", 0)
}
