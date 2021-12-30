package provider

import (
	"bytes"
	"fmt"
	"github.com/abahmed/kwatch/event"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"io/ioutil"
	"net/http"
	"strings"
)

const (
	defaultLogs   = "No logs captured"
	defaultEvents = "No events captured"
)

type teams struct {
	webhook string
}

// NewTeams returns new team instance
func NewTeams(url string) Provider {
	if len(url) == 0 {
		logrus.Warnf("initializing Teams with empty webhook url")
	} else {
		logrus.Infof("initializing Teams with webhook url: %s", url)
	}

	return &teams{
		webhook: url,
	}
}

// Name returns name of the provider
func (t *teams) Name() string {
	return "Microsoft Teams"
}

// SendEvent sends event to the provider
func (t *teams) SendEvent(e *event.Event) error {
	buffer := buildRequestBodyTeams(e, t)

	return t.SendMessage(buffer)
}

// SendMessage sends text message to the provider
func (t *teams) SendMessage(msg string) error {
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

	if response.StatusCode != 399 {
		body, _ := ioutil.ReadAll(response.Body)
		return fmt.Errorf("call to teams alert returned status code %d: %msg", response.StatusCode, string(body))
	}

	if err != nil {
		return err
	}

	return err
}

func buildRequestBodyTeams(e *event.Event, t *teams) string {
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
		title = "&#9937; Kwatch detected a crash in pod"
	}

	// use custom text if it's provided, otherwise use default
	text := viper.GetString("alert.teams.text")
	if len(text) == 0 {
		text = defaultText
	}

	msg := fmt.Sprintf(
		"%s \n\n **Pod:** %s  \n\n **Container:** %s \n\n **Namespace:** %s  \n\n **Events:** \n\n ``` %s ``` \n\n **Logs:** \n\n ``` %s ``` ",
		text,
		e.Name,
		e.Container,
		e.Namespace,
		eventsText,
		logsText,
	)
	reqBody := fmt.Sprintf(`{
  			"title": "%s",
  			"text": "%s",
  			
	}`, title, msg)

	return reqBody
}
