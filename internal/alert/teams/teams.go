package teams

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/safetext"
	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/notification"
)

const (
	defaultTeamsTitle = "Kwatch incident"
)

type Teams struct {
	sender transport.Sender
	// The HTTP trigger URL for the Power Automate flow
	webhook     string
	title       string
	clockSource clock.Clock

	// reference for general app configuration
	clusterName string
}

type teamsFlowPayload struct {
	Title      string                   `json:"title"`
	Text       string                   `json:"text"`
	Attachment []map[string]interface{} `json:"attachments"`
}

// NewTeams returns new team instance

func NewTeams(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Teams {
	webhook, ok := config["webhook"].(string)
	if !ok || len(webhook) == 0 {
		klog.InfoS("initializing Teams with empty flow url")
		return nil
	}

	if !transport.ValidEndpoint(webhook) {
		klog.InfoS("initializing teams with an invalid webhook",
			"setting", "webhook")
		return nil
	}

	klog.InfoS("initializing Teams with flow url configured")

	title, _ := config["title"].(string)

	return &Teams{
		sender:      transport.NewSender(dependencies),
		webhook:     webhook,
		title:       title,
		clusterName: clusterName,
		clockSource: clock.Require(dependencies.Clock),
	}
}

// Name returns name of the provider
func (t *Teams) Name() string {
	return "Microsoft Teams"
}

// SendIncident sends one incident message to the Power Automate flow.
func (t *Teams) SendIncident(
	ctx context.Context, m notification.Message,
) error {
	b, err := t.buildRequestBodyTeams(m)
	if err != nil {
		return err
	}
	return t.sendAPI(ctx, b)
}

// SendMessage sends plain text message to the Power Automate flow
func (t *Teams) SendMessage(ctx context.Context, msg string) error {
	b, err := t.buildRequestBodyMessage(msg)
	if err != nil {
		return err
	}
	return t.sendAPI(ctx, b)
}

// SendApi send the given payload to the Power Automate flow with retry logic
func (t *Teams) sendAPI(ctx context.Context, payload []byte) error {
	body, err := t.sender.Send(ctx, transport.Request{
		Provider: "Teams", URL: t.webhook, Body: payload,
	})
	if err != nil &&
		strings.Contains(string(body), "TriggerInputSchemaMismatch") {
		// The flow's trigger schema does not accept our payload; no retry
		// will change that.
		return transport.Permanent(
			fmt.Errorf(
				"failed to send message due to schema mismatch: %s",
				string(body),
			),
		)
	}
	return err
}

// buildRequestBodyTeams builds the incident payload: the configured or
// incident title, and the narrative as the text and as one card. The
// title carries no marker, so the narrative's marker is the only emoji.
func (t *Teams) buildRequestBodyTeams(
	m notification.Message,
) ([]byte, error) {
	title := t.title
	if len(title) == 0 {
		title = format.OrDefault(m.Title, defaultTeamsTitle)
	}
	text := safetext.RichWithOutput(m,
		notification.MarkdownDialect(nil, "\n\n"), "\n\n", 0)
	payload := &teamsFlowPayload{
		Title: title, Text: text,
		Attachment: textCardAttachments(title, text),
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
	// "Post card" flows iterate the attachments, so an empty list showed
	// nothing while Teams still answered 200. Always send one text card.
	payload := &teamsFlowPayload{
		Title:      "New Alert",
		Text:       msg,
		Attachment: textCardAttachments("New Alert", msg),
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

// textCardAttachments wraps plain text in one adaptive card.
func textCardAttachments(title, text string) []map[string]interface{} {
	return []map[string]interface{}{{
		"contentType": "application/vnd.microsoft.card.adaptive",
		"content": map[string]interface{}{
			"$schema": "http://adaptivecards.io/schemas/adaptive-card.json",
			"type":    "AdaptiveCard",
			"version": "1.2",
			"body": []map[string]interface{}{
				{"type": "TextBlock", "text": title, "weight": "Bolder",
					"wrap": true},
				{"type": "TextBlock", "text": text, "wrap": true},
			},
		},
	}}
}
