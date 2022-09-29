package rocketchat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/abahmed/kwatch/constant"
	"github.com/abahmed/kwatch/event"
	"github.com/sirupsen/logrus"
)

type RocketChat struct {
	webhook string
	text    string
}

type rocketChatWebhookPayload struct {
	Text string `json:"text"`
}

// NewRocketChat returns new rocket chat instance
func NewRocketChat(config map[string]string) *RocketChat {
	webhook, ok := config["webhook"]
	if !ok || len(webhook) == 0 {
		logrus.Warnf("initializing Rocket Chat with empty webhook url")
		return nil
	}

	logrus.Infof("initializing Rocket Chat with webhook url: %s", webhook)

	return &RocketChat{
		webhook: webhook,
		text:    config["text"],
	}
}

// Name returns name of the provider
func (r *RocketChat) Name() string {
	return "Rocket Chat"
}

// SendEvent sends event to the provider
func (r *RocketChat) SendEvent(e *event.Event) error {
	return r.sendByRocketChatApi(r.buildRequestBodyRocketChat(e, ""))
}

func (r *RocketChat) sendByRocketChatApi(reqBody string) error {
	client := &http.Client{}
	buffer := bytes.NewBuffer([]byte(reqBody))
	request, err := http.NewRequest(http.MethodPost, r.webhook, buffer)
	if err != nil {
		return err
	}

	request.Header.Set("Content-Type", "application/json")

	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode != 200 {
		body, _ := io.ReadAll(response.Body)
		return fmt.Errorf(
			"call to rocket chat alert returned status code %d: %s",
			response.StatusCode,
			string(body))
	}

	return nil
}

// SendMessage sends text message to the provider
func (r *RocketChat) SendMessage(msg string) error {
	return r.sendByRocketChatApi(
		r.buildRequestBodyRocketChat(new(event.Event), msg),
	)
}

func (r *RocketChat) buildRequestBodyRocketChat(
	e *event.Event,
	customMsg string) string {
	// add events part if it exists
	eventsText := constant.DefaultEvents
	events := strings.TrimSpace(e.Events)
	if len(events) > 0 {
		eventsText = e.Events
	}

	// add logs part if it exist
	logsText := constant.DefaultLogs
	logs := strings.TrimSpace(e.Logs)
	if len(logs) > 0 {
		logsText = e.Logs
	}

	// build text will be sent in the message use custom text if it's provided,
	// otherwise use default
	text := r.text
	if len(customMsg) <= 0 {
		text = fmt.Sprintf(
			"%s\n"+
				"**Pod:** %s\n"+
				"**Container:** %s\n"+
				"**Namespace:** %s\n"+
				"**Reason:** %s\n"+
				"**Events:**\n```\n%s\n```\n"+
				"**Logs:**\n```\n%s\n```",
			text,
			e.Name,
			e.Container,
			e.Namespace,
			e.Reason,
			eventsText,
			logsText,
		)
	} else {
		text = customMsg
	}

	msgPayload := &rocketChatWebhookPayload{
		Text: text,
	}

	jsonBytes, _ := json.Marshal(msgPayload)
	return string(jsonBytes)
}
