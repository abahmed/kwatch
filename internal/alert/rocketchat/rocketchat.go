package rocketchat

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

type RocketChat struct {
	webhook string
	text    string

	// reference for general app configuration
	appCfg *config.App
}

type rocketChatWebhookPayload struct {
	Text string `json:"text"`
}

// NewRocketChat returns new rocket chat instance
func NewRocketChat(
	config map[string]interface{},
	appCfg *config.App,
) *RocketChat {
	webhook, ok := config["webhook"].(string)
	if !ok || len(webhook) == 0 {
		klog.InfoS("initializing Rocket Chat with empty webhook url")
		return nil
	}

	klog.InfoS("initializing Rocket Chat with webhook configured")

	text, _ := config["text"].(string)

	return &RocketChat{
		webhook: webhook,
		text:    text,
		appCfg:  appCfg,
	}
}

// Name returns name of the provider
func (r *RocketChat) Name() string {
	return "Rocket Chat"
}

// SendEvent sends event to the provider
func (r *RocketChat) SendEvent(e *event.Event) error {
	formattedMsg := e.FormatMarkdown(r.appCfg.ClusterName, r.text, "")
	b, err := r.buildRequestBodyRocketChat(formattedMsg)
	if err != nil {
		return err
	}
	return r.sendByRocketChatApi(b)
}

func (r *RocketChat) sendByRocketChatApi(reqBody []byte) error {
	_, err := util.Send(
		util.Request{Provider: "RocketChat", URL: r.webhook, Body: reqBody},
	)
	return err
}

// SendMessage sends text message to the provider
func (r *RocketChat) SendMessage(msg string) error {
	b, err := r.buildRequestBodyRocketChat(msg)
	if err != nil {
		return err
	}
	return r.sendByRocketChatApi(b)
}

// SendIncident implements alert.ThreadProvider.
// It renders the incident using the Report model and PlaintextRenderer,
// producing a context-adaptive text message.
func (r *RocketChat) SendIncident(
	inc *model.Incident,
	action model.IncidentAction,
) error {
	return r.SendIncidentWithInsight(inc, action, nil)
}

// SendIncidentWithInsight implements alert.InsightThreadProvider, so the
// diagnosis — likely cause, impact, recent changes — is rendered rather than
// dropped on the way to this provider.
func (r *RocketChat) SendIncidentWithInsight(
	inc *model.Incident,
	action model.IncidentAction,
	ins *insight.Insight,
) error {
	text := util.RenderIncidentWithInsight(
		inc,
		action,
		ins,
		message.NewPlainTextRenderer(),
		r.appCfg.ClusterName,
	)
	if text == "" {
		return nil
	}
	return r.SendMessage(text)
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
