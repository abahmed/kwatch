package rocketchat

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/message"
	"github.com/abahmed/kwatch/internal/model"
)

type RocketChat struct {
	sender  transport.Sender
	webhook string
	text    string

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

	klog.InfoS("initializing Rocket Chat with webhook configured")

	text, _ := config["text"].(string)

	return &RocketChat{
		sender:      transport.NewSender(dependencies),
		webhook:     webhook,
		text:        text,
		clusterName: clusterName,
		clockSource: clock.Require(dependencies.Clock),
	}
}

// Name returns name of the provider
func (r *RocketChat) Name() string {
	return "Rocket Chat"
}

// SendEvent sends event to the provider
func (r *RocketChat) SendEvent(ctx context.Context, e *event.Event) error {
	formattedMsg := e.FormatMarkdown(r.clusterName, r.text, "")
	b, err := r.buildRequestBodyRocketChat(formattedMsg)
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

// SendIncident implements delivery.ThreadProvider.
// It renders the incident using the Report model and PlaintextRenderer,
// producing a context-adaptive text message.
func (r *RocketChat) SendIncident(
	ctx context.Context,
	inc *model.Incident,
	action model.IncidentAction,
) error {
	return r.SendIncidentWithInsight(ctx, inc, action, nil)
}

// SendIncidentWithInsight implements delivery.InsightThreadProvider, so the
// diagnosis — likely cause, impact, recent changes — is rendered rather than
// dropped on the way to this provider.
func (r *RocketChat) SendIncidentWithInsight(
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
		r.clusterName,
		r.clockSource,
	)
	if text == "" {
		return nil
	}
	return r.SendMessage(ctx, text)
}

func (r *RocketChat) buildRequestBodyRocketChat(text string) ([]byte, error) {
	msgPayload := &rocketChatWebhookPayload{
		Text: text,
	}

	jsonBytes, err := json.Marshal(msgPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal rocketchat payload: %w", err)
	}
	return jsonBytes, nil
}
