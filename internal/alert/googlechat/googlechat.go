package googlechat

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/message"
	"github.com/abahmed/kwatch/internal/model"
)

type GoogleChat struct {
	sender  transport.Sender
	webhook string
	text    string

	// reference for general app configuration
	clusterName string
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

	klog.InfoS("initializing Google Chat with webhook configured")

	text, _ := config["text"].(string)

	return &GoogleChat{
		sender:      transport.NewSender(dependencies),
		webhook:     webhook,
		text:        text,
		clusterName: clusterName,
	}
}

// Name returns name of the provider
func (g *GoogleChat) Name() string {
	return "Google Chat"
}

// SendEvent sends event to the provider
func (g *GoogleChat) SendEvent(ctx context.Context, e *event.Event) error {
	formattedMsg := e.FormatText(g.clusterName, g.text)
	b, err := g.buildRequestBody(formattedMsg)
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

// SendIncident implements delivery.ThreadProvider.
// It renders the incident using the Report model and PlaintextRenderer,
// producing a context-adaptive text message.
func (g *GoogleChat) SendIncident(
	ctx context.Context,
	inc *model.Incident,
	action model.IncidentAction,
) error {
	return g.SendIncidentWithInsight(ctx, inc, action, nil)
}

// SendIncidentWithInsight implements delivery.InsightThreadProvider, so the
// diagnosis — likely cause, impact, recent changes — is rendered rather than
// dropped on the way to this provider.
func (g *GoogleChat) SendIncidentWithInsight(
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
		g.clusterName,
	)
	if text == "" {
		return nil
	}
	return g.SendMessage(ctx, text)
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
