package teams

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"net/http"
	"strings"

	"github.com/abahmed/kwatch/event"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

const (
	defaultLogs       = "No logs captured"
	defaultEvents     = "No events captured"
	defaultTeamsTitle = "&#9937; Kwatch detected a crash in pod"
	defaultTitle      = ":red_circle: kwatch detected a crash in pod"
	defaultText       = "There is an issue with container in a pod!"
)

type Teams struct {
	webhook string
}

type teamsWebhookPayload struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}

// NewTeams returns new team instance
func NewTeams(config map[string]string) *Teams {
	webhook, ok := config["webhook"]

	if !ok || len(webhook) == 0 {
		logrus.Warnf("initializing Teams with empty webhook url")
		return nil
	}

	logrus.Infof("initializing Teams with webhook url: %s", webhook)

	return &Teams{
		webhook: webhook,
	}
}

// Name returns name of the provider
func (t *Teams) Name() string {
	return "Microsoft Teams"
}

// SendEvent sends event to the provider
func (t *Teams) SendEvent(e *event.Event) error {
	return t.SendMessage(t.buildRequestBodyTeams(e))
}

// SendMessage sends text message to the provider
func (t *Teams) SendMessage(msg string) error {
	client := &http.Client{}
	buffer := bytes.NewBuffer([]byte(msg))
	request, err := http.NewRequest(http.MethodPost, t.webhook, buffer)
	if err != nil {
		return err
	}

	request.Header.Set("Content-Type", "application/json")

	response, err := client.Do(request)
	if err != nil {
		return err
	}

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
func (t *Teams) buildRequestBodyTeams(e *event.Event) string {
	eventsText := defaultEvents
	logsText := defaultLogs

	// add events part if it exists
	events := strings.TrimSpace(e.Events)
	if len(events) > 0 {
		eventsText = e.Events
	}

	// add logs part if it exists
	logs := strings.TrimSpace(e.Logs)
	if len(logs) > 0 {
		logsText = e.Logs
	}
	// use custom title if it's provided, otherwise use default
	title := viper.GetString("alert.teams.title")
	if len(title) == 0 {
		title = defaultTeamsTitle
	}

	// use custom text if it's provided, otherwise use default
	text := viper.GetString("alert.teams.text")
	if len(text) == 0 {
		text = defaultText
	}

	msg := fmt.Sprintf(
		"%s \n\n "+
			"**Pod:** %s  \n\n "+
			"**Container:** %s \n\n "+
			"**Namespace:** %s  \n\n "+
			"**Events:** \n\n ``` %s ``` \n\n "+
			"**Logs:** \n\n ``` %s ``` ",
		text,
		e.Name,
		e.Container,
		e.Namespace,
		eventsText,
		logsText,
	)
	msgPayload := &teamsWebhookPayload{
		Title: title,
		Text:  msg,
	}

	jsonBytes, _ := json.Marshal(msgPayload)
	return string(jsonBytes)
}
