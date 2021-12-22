package provider

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/abahmed/kwatch/event"
	"github.com/sirupsen/logrus"
)

const (
	telegramAPIURL = "https://api.telegram.org/bot%s/sendMessage"
)

type telegram struct {
	token string
}

// NewTelegram returns a new Telegram object
func NewTelegram(token string) Provider {
	if len(token) == 0 {
		logrus.Warnf("initializing telegram with empty token")
	} else {
		logrus.Infof("initializing telegram with token  %s", token)
	}
	return &telegram{
		token: token,
	}
}

// Name returns name of the provider
func (t telegram) Name() string {
	return "Telegram"
}

// SendEvent sends event to the provider
func (t telegram) SendEvent(e *event.Event) error {
	logrus.Debugf("sending to telegram event: %v", e)

	if len(t.token) == 0 {
		return errors.New("token key is empty")
	}

	client := &http.Client{}

	reqBody := buildRequestBodyTelegram(e, t.token)
	buffer := bytes.NewBuffer([]byte(reqBody))

	request, err := http.NewRequest(http.MethodPost, telegramAPIURL, buffer)
	if err != nil {
		return err
	}

	request.Header.Set("Content-Type", "application/json")

	response, err := client.Do(request)
	if err != nil || response.StatusCode > 202 {
		return err
	}

	return nil
}

// SendMessage sends text message to the provider
func (t telegram) SendMessage(s string) error {
	return nil
}

func buildRequestBodyTelegram(e *event.Event, token string) string {
	eventsText := "No events captured"
	logsText := "No logs captured"

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

	reqBody := fmt.Sprintf(`{
		"routing_key": "%s",
		"event_action": "trigger",
		"payload": {
		  "summary": "%s",
		  "source": "%s",
		  "severity": "critical",
		  "custom_details": {
			"Name": "%s",
			"Container": "%s",
			"Namespace": "%s",
			"Reason": "%s",
			"Events": "%s",
			"Logs": "%s"
		  }
		}
	  }`,
		token,
		fmt.Sprintf(defaultEventTitle, e.Container),
		e.Container,
		e.Name,
		e.Container,
		e.Namespace,
		e.Reason,
		eventsText,
		logsText)

	return reqBody
}
