package teams

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/k8s"
	"k8s.io/klog/v2"
)

const (
	defaultTeamsTitle = "&#9937; Kwatch detected a crash in pod"
	defaultMaxRetries = 3
	defaultRetryDelay = 5
)

type Teams struct {
	// The HTTP trigger URL for the Power Automate flow
	webhook    string
	title      string
	text       string
	maxRetries int
	retryDelay int

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

	klog.InfoS("initializing Teams with flow url", "webhook", webhook)

	title, _ := config["title"].(string)
	text, _ := config["text"].(string)

	maxRetries, mxOk := config["maxRetries"].(int)
	if !mxOk || maxRetries == 0 {
		maxRetries = defaultMaxRetries
	}

	retryDelay, dlOk := config["retryDelay"].(int)
	if !dlOk || retryDelay == 0 {
		retryDelay = defaultRetryDelay
	}

	return &Teams{
		webhook:    webhook,
		title:      title,
		text:       text,
		maxRetries: maxRetries,
		retryDelay: retryDelay,
		appCfg:     appCfg,
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

// SendApi send the given payload to the Power Automate flow with retry logic
func (t *Teams) sendAPI(payload []byte) error {
	// try to send the message up to "maxRetries" times
	for attempts := 0; attempts < t.maxRetries; attempts++ {
		request, err :=
			http.NewRequest(
				http.MethodPost,
				t.webhook,
				bytes.NewBuffer(payload))
		if err != nil {
			return fmt.Errorf("error creating HTTP request: %v", err)
		}

		request.Header.Set("Content-Type", "application/json")

		client := k8s.GetDefaultClient()
		resp, err := client.Do(request)
		if err != nil {
			return fmt.Errorf("failed to create HTTP response: %w", err)
		}
		if resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return nil
		}

		if resp.StatusCode == http.StatusBadRequest {
			body, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			if readErr != nil {
				return fmt.Errorf("call to power automate flow returned status %d", resp.StatusCode)
			}
			if strings.Contains(string(body), "TriggerInputSchemaMismatch") {
				return fmt.Errorf(
					"failed to send message due to schema mismatch: %s",
					string(body))
			}
			return fmt.Errorf(
				"call to power automate flow returned status %d: %s",
				resp.StatusCode,
				string(body))
		}

		if resp.StatusCode == http.StatusAccepted {
			resp.Body.Close()
			klog.InfoS("Request accepted by Power Automate flow, but not processed immediately",
				"attempt", attempts+1,
				"maxRetries", t.maxRetries)
		} else {
			body, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			if readErr != nil {
				return fmt.Errorf(
					"call to power automate flow returned status %d", resp.StatusCode)
			}
			return fmt.Errorf(
				"call to power automate flow returned status %d: %s",
				resp.StatusCode,
				string(body))
		}

		// Wait for a delay before retrying
		if attempts < t.maxRetries-1 {
			time.Sleep(time.Duration(t.retryDelay) * time.Second)
		}
	}

	// After all retries, return an error
	return fmt.Errorf("failed to send message after %d attempts", t.maxRetries)
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
					body := []map[string]interface{}{
						{
							"type": "TextBlock",
							"text": title,
						},
						{
							"type": "TextBlock",
							"text": fmt.Sprintf("Pod Name: %s", e.PodName),
						},
						{
							"type": "TextBlock",
							"text": fmt.Sprintf("Namespace: %s", e.Namespace),
						},
						{
							"type": "TextBlock",
							"text": fmt.Sprintf("Node: %s", e.NodeName),
						},
						{
							"type": "TextBlock",
							"text": fmt.Sprintf("Reason: %s", e.Reason),
						},
					}
					if e.IncludeLogs {
						body = append(body, map[string]interface{}{
							"type": "TextBlock",
							"text": fmt.Sprintf("Logs: %s", e.Logs),
						})
					}
					if e.IncludeEvents {
						body = append(body, map[string]interface{}{
							"type": "TextBlock",
							"text": fmt.Sprintf("Events: \n%s", e.Events),
						})
					}
					body = append(body, map[string]interface{}{
						"type": "TextBlock",
						"text": fmt.Sprintf(
							"Time: %s",
							time.Now().Format(time.RFC1123)),
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
		return nil, fmt.Errorf("failed to marshal teams message payload: %w", err)
	}

	return jsonBytes, nil
}
