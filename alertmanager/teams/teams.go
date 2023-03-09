package teams

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"net/http"

	"github.com/abahmed/kwatch/config"
	"github.com/abahmed/kwatch/event"
	"github.com/sirupsen/logrus"
)

const (
	defaultTeamsTitle = "&#9937; Kwatch detected a crash in pod"
)

type Teams struct {
	webhook string
	title   string
	text    string

	// reference for general app configuration
	appCfg *config.App
}

type teamsWebhookPayload struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}

// NewTeams returns new team instance
func NewTeams(config map[string]interface{}, appCfg *config.App) *Teams {
	webhook, ok := config["webhook"].(string)
	if !ok || len(webhook) == 0 {
		logrus.Warnf("initializing Teams with empty webhook url")
		return nil
	}

	logrus.Infof("initializing Teams with webhook url: %s", webhook)

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

// SendEvent sends event to the provider
func (t *Teams) SendEvent(e *event.Event) error {
	return t.sendAPI(t.buildRequestBodyTeams(e))
}

// SendMessage sends text message to the provider
func (t *Teams) SendMessage(msg string) error {

	msgPayload := &teamsWebhookPayload{
		Text: msg,
	}

	jsonBytes, _ := json.Marshal(msgPayload)
	return t.sendAPI(jsonBytes)
}

func (t *Teams) sendAPI(b []byte) error {
	buffer := bytes.NewBuffer(b)
	request, err := http.NewRequest(http.MethodPost, t.webhook, buffer)
	if err != nil {
		return err
	}

	request.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode != 200 {
		body, _ := io.ReadAll(response.Body)
		return fmt.Errorf(
			"call to teams alert returned status code %d: %s",
			response.StatusCode,
			string(body))
	}

	return nil
}

// buildRequestBodyTeams builds formatted string from event
func (t *Teams) buildRequestBodyTeams(e *event.Event) []byte {
	// use custom title if it's provided, otherwise use default
	title := t.title
	if len(title) == 0 {
		title = defaultTeamsTitle
	}

	msg := e.FormatMarkdown(t.appCfg.ClusterName, t.text, "\n\n")
	msgPayload := &teamsWebhookPayload{
		Title: title,
		Text:  msg,
	}

	jsonBytes, _ := json.Marshal(msgPayload)
	return (jsonBytes)
}
