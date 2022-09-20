package provider

import (
	"bytes"
	"encoding/json"
	"errors"
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

type rocketChat struct {
	webhook string
}

type rocketChatWebhookPayload struct {
	Text string `json:"text"`
}

// NewRocketChat returns new rocket chat instance
func NewRocketChat(config map[string]string) Provider {
	webhook, ok := config["webhook"]
	if !ok || len(webhook) == 0 {
		logrus.Warnf("initializing Rocket Chat with empty webhook url")
		return nil
	}

	logrus.Infof("initializing Rocket Chat with webhook url: %s", webhook)

	return &slack{
		webhook: webhook,
	}
}

// Name returns name of the provider
func (r *rocketChat) Name() string {
	return "Rocket Chat"
}

// SendEvent sends event to the provider
func (r *rocketChat) SendEvent(e *event.Event) error {
	logrus.Debugf("sending to rocket chat event: %v", e)

	// validate rocket chat webhook url
	_, err := validateRocketChat(r)
	if err != nil {
		return err
	}
	reqBody, err := buildRequestBodyRocketChat(e, "")
	if err != nil {
		return err
	}
	return sendByRocketChatApi(reqBody, r)
}

func sendByRocketChatApi(reqBody string, r *rocketChat) error {
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

	if response.StatusCode != 200 {
		body, _ := io.ReadAll(response.Body)
		return fmt.Errorf(
			"call to rocket chat alert returned status code %d: %s",
			response.StatusCode,
			string(body))
	}

	if err != nil {
		return err
	}

	return err
}

func buildRequestBodyRocketChat(e *event.Event, customMsg string) (string, error) {
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

	jsonBytes, err := json.Marshal(msgPayload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal string %v: %s", msgPayload, err)
	}

	return string(jsonBytes), nil
}

// SendMessage sends text message to the provider
func (r *rocketChat) SendMessage(msg string) error {
	logrus.Debugf("sending to rocket chat msg: %s", msg)

	// validate rocket chat webhook url
	_, err := validateRocketChat(r)
	if err != nil {
		return err
	}

	reqBody, err := buildRequestBodyRocketChat(new(event.Event), msg)
	if err != nil {
		return err
	}
	return sendByRocketChatApi(reqBody, r)
}

func validateRocketChat(r *rocketChat) (bool, error) {
	if len(r.webhook) == 0 {
		return false, errors.New("webhook url is empty")
	}
	return true, nil
}
