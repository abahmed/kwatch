package teamsworkflow

import (
	"encoding/json"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
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
	webhook string

	appCfg *config.App
}

// NewTeamsWorkflow returns a new TeamsWorkflow object
func NewTeamsWorkflow(config map[string]interface{}, appCfg *config.App) *TeamsWorkflow {
	webhook, ok := config["webhook"].(string)
	if !ok || len(webhook) == 0 {
		klog.InfoS("initializing teams workflow with empty webhook url")
		return nil
	}

	klog.InfoS("initializing Teams Workflow with webhook url")

	return &TeamsWorkflow{
		webhook: webhook,
		appCfg:  appCfg,
	}
}

// Name returns name of the provider
func (t *TeamsWorkflow) Name() string {
	return "Teams Workflow"
}

// SendEvent sends event to the provider
func (t *TeamsWorkflow) SendEvent(e *event.Event) error {
	msg := e.FormatMarkdown(t.appCfg.ClusterName, "", "\n\n")
	return t.SendMessage(msg)
}

// SendMessage sends text message to the provider
func (t *TeamsWorkflow) SendMessage(msg string) error {
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

	_, err = util.Post(t.Name(), t.webhook, body, "application/json", nil)
	return err
}
