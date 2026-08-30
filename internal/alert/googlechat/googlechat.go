package googlechat

import (
	"encoding/json"
	"fmt"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/message"
	"github.com/abahmed/kwatch/internal/model"
)

type GoogleChat struct {
	webhook string
	text    string

	// reference for general app configuration
	appCfg *config.App
}

type payload struct {
	Text string `json:"text"`
}

// NewGoogleChat returns new google chat instance
func NewGoogleChat(
	config map[string]interface{},
	appCfg *config.App,
) *GoogleChat {
	webhook, ok := config["webhook"].(string)
	if !ok || len(webhook) == 0 {
		klog.InfoS("initializing Google Chat with empty webhook url")
		return nil
	}

	klog.InfoS("initializing Google Chat with webhook configured")

	text, _ := config["text"].(string)

	return &GoogleChat{
		webhook: webhook,
		text:    text,
		appCfg:  appCfg,
	}
}

// Name returns name of the provider
func (g *GoogleChat) Name() string {
	return "Google Chat"
}

// SendEvent sends event to the provider
func (g *GoogleChat) SendEvent(e *event.Event) error {
	formattedMsg := e.FormatText(g.appCfg.ClusterName, g.text)
	b, err := g.buildRequestBody(formattedMsg)
	if err != nil {
		return err
	}
	return g.sendAPI(b)
}

func (g *GoogleChat) sendAPI(reqBody []byte) error {
	_, err := util.Send(
		util.Request{Provider: "GoogleChat", URL: g.webhook, Body: reqBody},
	)
	return err
}

// SendMessage sends text message to the provider
func (g *GoogleChat) SendMessage(msg string) error {
	b, err := g.buildRequestBody(msg)
	if err != nil {
		return err
	}
	return g.sendAPI(b)
}

// SendIncident implements alert.ThreadProvider.
// It renders the incident using the Report model and PlaintextRenderer,
// producing a context-adaptive text message.
func (g *GoogleChat) SendIncident(
	inc *model.Incident,
	action model.IncidentAction,
) error {
	return g.SendIncidentWithInsight(inc, action, nil)
}

// SendIncidentWithInsight implements alert.InsightThreadProvider, so the
// diagnosis — likely cause, impact, recent changes — is rendered rather than
// dropped on the way to this provider.
func (g *GoogleChat) SendIncidentWithInsight(
	inc *model.Incident,
	action model.IncidentAction,
	ins *insight.Insight,
) error {
	text := util.RenderIncidentWithInsight(
		inc,
		action,
		ins,
		message.NewPlainTextRenderer(),
		g.appCfg.ClusterName,
	)
	if text == "" {
		return nil
	}
	return g.SendMessage(text)
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
