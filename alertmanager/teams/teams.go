package teams

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
	"net/http"

	"github.com/abahmed/kwatch/config"
	"github.com/abahmed/kwatch/event"
	"github.com/sirupsen/logrus"
)

const (
	defaultTeamsTitle = "&#9937; Kwatch detected a crash in pod"
)

type Teams struct {
	# The HTTP trigger URL for the Power Automate flow
	flowUrl string
	title   string
	text    string

	// reference for general app configuration
	appCfg *config.App
}

type teamsFlowPayload struct {
	Title string `json:"title"`
	Text  string `json:"text"`
	Attachment []map[string]interface{} `json:"attachment"`
}

// NewTeams returns new team instance
func NewTeams(config map[string]interface{}, appCfg *config.App) *Teams {
	flowURL, ok := config["flowURL"].(string)
	if !ok || len(flowURL) == 0 {
		logrus.Warnf("initializing Teams with empty flow url")
		return nil
	}

	logrus.Infof("initializing Teams with flow url: %s", flowURL)

	title, _ := config["title"].(string)
	text, _ := config["text"].(string)

	return &Teams{
		flowURL: flowURL,
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
	payload := t.buildRequestBodyTeams(e)
	return t.sendAPI(payload)
}

// SendMessage sends plain text message to the Power Automate flow
func (t *Teams) SendMessage(msg string) error {
	payload := t.buildRequestBodyMessage(msg)
	return t.sendAPI(payload)
}

// SendApi send the given payload to the Power Automate flow with retry logic
func (t *Teams) sendAPI(payload []byte) error {
	// Number of retry attempts
	maxRetries := 3
	retryDelay := 5 * time.Second

	// try to send the message up to "maxretries" times
	for attemps := 0; attemps < maxRetries; attemps++ {
		request, err := http.NewRequest(http.MethodPost, t.flowURL, bytes.NewBuffer(payload))
		if err != nil {
			return fmt.Errorf("error creating HTTP request: %v", err)
		}

		request.Header.Set("Content-Type", "application/json")

		client := &http.Client{}
		resp, err := client.Do(request)
		if err != nil {
			return fmt.Errorf("failed to create HTTP response: %w", err)
		}
		defer resp.Body.Close()

		//Check for success (HTTP 200 OK)
		if resp.StatusCode == http.StatusOK {
			return nil
			// Message successfully sent		
		}

		//Handle specific 400 errors (TriggerInputSchemaMismatch)
		if resp.StatusCode == http.StatusBadRequest {
			body, _ := io.ReadAll(response.Body)
			//Check for error (TriggerInputSchemaMismatch)
			if strings.Contains(string(body), "TriggerInputSchemaMismatch") {
				return fmt.Errorf("failed to send message due to schema mismatch: %s", string(body))
			}
			return fmt.Errorf("call to power automate flow returned status %d: %s", resp.StatusCode, string(body))
		}

		//Handle 202 status and retry
		if response.StatusCode == http.StatusAccepted {
			logrus.Warnf("Request accepted by Power Automate flow, but not processed immediately.Attempt %d of %d.", attempts+1, maxRetries)
		}
		else {
			// For other non-200 status codes, log the error
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("call to power automate flow returned status %d: %s", resp.StatusCode, string(body))
		}

		//Wait for a delay before retrying
		if attempts < maxRetries-1 {
			time.Sleep(retryDelay)
		}
	}

	//After all retries, return an error
	return fmt .Errorf("failed to send message after %d attempts", maxRetries)
}

//buildRequestBodyTeams builds the request body for the Power Automate flow
func (t *Teams) buildRequestBodyTeams(e *event.Event) []byte {
	// Use custom title if its provided, otherwise use the default title
	title := t.title
	if len(title) == 0 {
		title = defaultTeamsTitle
	}

	//format the message with markdown
	msg : = e.FormatMarkdown(t.appCfg.ClusterName, t.text, "\n\n")

	// Create the attachment for the message with full event details
	attachments := []map[string]interface{}{
		{
			"contentType": "application/vnd.microsoft.card.adaptive",
			"content": map[string]interface{}{
				"$schema": "http://adaptivecards.io/schemas/adaptive-card.json",
				"type": "AdaptiveCard",
				"version": "1.2",
				"body": []map[string]interface{}{
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
						"text": fmt.Sprintf("Reason: %s", e.Reason),
					},
					{
						"type": "TextBlock",
						"text": fmt.Sprintf("Logs: %s", e.Logs),
					},
					{
						"type": "TextBlock",
						"text": fmt.Sprintf("Events: \n%s", e.Events),
					},
					{
						"type": "TextBlock",
						"text": fmt.Sprintf("Time: %s", time.now().Format(time.RFC1123Format)),
					},	

				},
			},
		},
	}

	//Prepare the payload for the Power Automate flow
	payload := &teamsFlowPayload{
		Title: title,
		Text:  msg,
		Attachment: attachments, // Attachment should be an array
	}

	jsonBytes, _ := json.Marshal(payload)
	return jsonBytes
}

//buildRequestBodyMessage builds plain message payload for the Power Automate flow
func (t *Teams) buildRequestBodyMessage(msg string) []byte {
	payload := &teamsFlowPayload{
		Title: "New Alert",
		Text:  msg,
	    // Empty attachments array to prevent schema mismatch error
		Attachment: []map[string]interface{}{},
	}

	jsonBytes, _ := json.Marshal(payload)
	return jsonBytes
}


