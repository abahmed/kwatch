package provider

import (
	"bytes"
	"fmt"
	"github.com/abahmed/kwatch/event"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"net/http"
	"strings"
)

type teams struct {
	webhook string
}

// NewDiscord returns new Discord instance
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
	return "Teams"
}

// SendEvent sends event to the provider
func (t *teams) SendEvent(e *event.Event) error {

	client := &http.Client{}
	buffer := bytes.NewBuffer([]byte(buildRequestBodyTeams(e, t)))
	request, err := http.NewRequest(http.MethodPost, t.webhook, buffer)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	if response.StatusCode > 399 {
		body, _ := ioutil.ReadAll(response.Body)
		return fmt.Errorf("call to provider alert returned status code %d: %s", response.StatusCode, string(body))
	}

	if err != nil {
		return err
	}

	return err
}

// SendMessage sends text message to the provider
func (t *teams) SendMessage(s string) error {
	return nil
}

func buildRequestBodyTeams(e *event.Event, t *teams) string {
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

	msg := fmt.Sprintf(
		"An alert for Name: *%s*  Container: *%s* Namespace: *%s*  has been triggered:\\n—\\n Logs: *%s* \\n Events: *%s* ",
		e.Name,
		e.Container,
		e.Namespace,
		logsText,
		eventsText,
	)
	reqBody := fmt.Sprintf(`{
  			"@type": "MessageCard",
  			"@context": "http://schema.org/extensions",
  			"title": &#9937;&#x26D1; Kwatch detected a crash in pod",
  			"text": "%s",
  			
	}`, msg)

	return reqBody
}
