package teams

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/message"
	"github.com/abahmed/kwatch/internal/model"
)

const (
	defaultTeamsTitle = "&#9937; Kwatch detected a crash in pod"
)

type Teams struct {
	// The HTTP trigger URL for the Power Automate flow
	webhook string
	title   string
	text    string

	// reference for general app configuration
	appCfg *config.App
}

type teamsFlowPayload struct {
	Title      string                   `json:"title"`
	Text       string                   `json:"text"`
	Attachment []map[string]interface{} `json:"attachments"`
}

// NewTeams returns new team instance
func NewTeams(config map[string]interface{}, appCfg *config.App) *Teams {
	webhook, ok := config["webhook"].(string)
	if !ok || len(webhook) == 0 {
		klog.InfoS("initializing Teams with empty flow url")
		return nil
	}

	klog.InfoS("initializing Teams with flow url configured")

	title, _ := config["title"].(string)
	text, _ := config["text"].(string)

	return &Teams{
		webhook: webhook,
		title:   title,
		text:    text,
		appCfg:  appCfg,
	}
}

// Name returns name of the provider
func (t *Teams) Name() string {
	return "Microsoft Teams"
}

// SendEvent sends event to the Power Automate flow
func (t *Teams) SendEvent(e *event.Event) error {
	b, err := t.buildRequestBodyTeams(e)
	if err != nil {
		return err
	}
	return t.sendAPI(b)
}

// SendMessage sends plain text message to the Power Automate flow
func (t *Teams) SendMessage(msg string) error {
	b, err := t.buildRequestBodyMessage(msg)
	if err != nil {
		return err
	}
	return t.sendAPI(b)
}

// SendIncident implements alert.ThreadProvider.
// It renders the incident using the Report model and PlaintextRenderer,
// producing a context-adaptive text message.
func (t *Teams) SendIncident(
	inc *model.Incident,
	action model.IncidentAction,
) error {
	return t.SendIncidentWithInsight(inc, action, nil)
}

// SendIncidentWithInsight implements alert.InsightThreadProvider, so the
// diagnosis — likely cause, impact, recent changes — is rendered rather than
// dropped on the way to this provider.
func (t *Teams) SendIncidentWithInsight(
	inc *model.Incident,
	action model.IncidentAction,
	ins *insight.Insight,
) error {
	text := util.RenderIncidentWithInsight(
		inc,
		action,
		ins,
		message.NewPlainTextRenderer(),
		t.appCfg.ClusterName,
	)
	if text == "" {
		return nil
	}
	return t.SendMessage(text)
}

// SendApi send the given payload to the Power Automate flow with retry logic
func (t *Teams) sendAPI(payload []byte) error {
	body, err := util.Send(
		util.Request{Provider: "Teams", URL: t.webhook, Body: payload},
	)
	if err != nil &&
		strings.Contains(string(body), "TriggerInputSchemaMismatch") {
		// The flow's trigger schema does not accept our payload; no retry
		// will change that.
		return event.Permanent(
			fmt.Errorf(
				"failed to send message due to schema mismatch: %s",
				string(body),
			),
		)
	}
	return err
}

// buildRequestBodyTeams builds the request body for the Power Automate flow
func (t *Teams) buildRequestBodyTeams(e *event.Event) ([]byte, error) {
	// Use custom title if it's provided, otherwise use the default title
	title := t.title
	if len(title) == 0 {
		title = defaultTeamsTitle
	}

	// Format the message with markdown
	msg := e.FormatMarkdown(t.appCfg.ClusterName, t.text, "\n\n")

	// Create the attachment for the message with full event details
	attachments := []map[string]interface{}{
		{
			"contentType": "application/vnd.microsoft.card.adaptive",
			"content": map[string]interface{}{
				"$schema": "http://adaptivecards.io/schemas/adaptive-card.json",
				"type":    "AdaptiveCard",
				"version": "1.2",
				"body": func() []map[string]interface{} {
					body := []map[string]interface{}{}
					body = append(body, map[string]interface{}{
						"type": "TextBlock",
						"text": title,
					})
					if e.PodName != "" {
						body = append(body, map[string]interface{}{
							"type": "TextBlock",
							"text": fmt.Sprintf("Pod Name: %s", e.PodName),
						})
					}
					if e.Namespace != "" {
						body = append(body, map[string]interface{}{
							"type": "TextBlock",
							"text": fmt.Sprintf("Namespace: %s", e.Namespace),
						})
					}
					if e.NodeName != "" {
						body = append(body, map[string]interface{}{
							"type": "TextBlock",
							"text": fmt.Sprintf("Node: %s", e.NodeName),
						})
					}
					if e.Reason != "" {
						body = append(body, map[string]interface{}{
							"type": "TextBlock",
							"text": fmt.Sprintf("Reason: %s", e.Reason),
						})
					}
					if e.IncludeLogs && strings.TrimSpace(e.Logs) != "" {
						body = append(body, map[string]interface{}{
							"type": "TextBlock",
							"text": fmt.Sprintf(
								"Logs: %s",
								strings.TrimSpace(e.Logs),
							),
						})
					}
					if e.IncludeEvents && strings.TrimSpace(e.Events) != "" {
						body = append(body, map[string]interface{}{
							"type": "TextBlock",
							"text": fmt.Sprintf(
								"Events: \n%s",
								strings.TrimSpace(e.Events),
							),
						})
					}
					body = append(body, map[string]interface{}{
						"type": "TextBlock",
						"text": fmt.Sprintf(
							"Time: %s",
							clock.Now().Format(time.RFC1123)),
					})
					return body
				}(),
			},
		},
	}

	// Prepare the payload for the Power Automate flow
	payload := &teamsFlowPayload{
		Title:      title,
		Text:       msg,
		Attachment: attachments, // Attachment should be an array
	}

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal teams event payload: %w", err)
	}

	return jsonBytes, nil
}

// buildRequestBodyMessage builds plain message payload for the Power
// Automate flow
func (t *Teams) buildRequestBodyMessage(msg string) ([]byte, error) {
	payload := &teamsFlowPayload{
		Title: "New Alert",
		Text:  msg,
		// Empty attachments array to prevent schema mismatch error
		Attachment: []map[string]interface{}{},
	}

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to marshal teams message payload: %w",
			err,
		)
	}

	return jsonBytes, nil
}
