package rocketchat

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
	defaultRocketChatLogs   = "No logs captured"
	defaultRocketChatEvents = "No events captured"
)

type RocketChat struct {
	webhook string
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
	}
}

// Name returns name of the provider
func (r *RocketChat) Name() string {
	return "Rocket Chat"
}

// SendEvent sends event to the provider
func (r *RocketChat) SendEvent(e *event.Event) error {
	return r.sendByRocketChatApi(buildRequestBodyRocketChat(e, ""))
}

func (r *RocketChat) sendByRocketChatApi(reqBody string) error {
	client := &http.Client{}
	buffer := bytes.NewBuffer([]byte(reqBody))
	request, _ := http.NewRequest(http.MethodPost, r.webhook, buffer)
	request.Header.Set("Content-Type", "application/json")

	response, _ := client.Do(request)
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
		buildRequestBodyRocketChat(new(event.Event), msg),
	)
}

func buildRequestBodyRocketChat(e *event.Event, customMsg string) string {
	eventsText := defaultRocketChatEvents
	logsText := defaultRocketChatLogs

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

	// build text will be sent in the message use custom text if it's provided,
	// otherwise use default
	text := viper.GetString("alert.rocketchat.text")
	if len(customMsg) <= 0 {
		text = fmt.Sprintf(
			"%s \n\n **Pod:** %s  \n\n **Container:** %s \n\n **Namespace:** %s  \n\n **Events:** \n\n ``` %s ``` \n\n **Logs:** \n\n ``` %s ``` ",
			text,
			e.Name,
			e.Container,
			e.Namespace,
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
