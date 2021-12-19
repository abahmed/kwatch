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
	pagerdutyAPIURL   = "https://events.pagerduty.com/v2/enqueue"
	defaultEventTitle = "[%s] There is an issue with a container in a pod"
)

type pagerduty struct {
	integrationKey string
}

func NewPagerDuty(integrationKey string) Provider {
	if len(integrationKey) == 0 {
		logrus.Warnf("initializing pagerduty with an empty integration key")
	} else {
		logrus.Infof("initializing pagerduty with the provided integration key")
	}

	return &pagerduty{
		integrationKey: integrationKey,
	}
}

// Name returns name of the provider
func (s *pagerduty) Name() string {
	return "PagerDuty"
}

// SendEvent sends event to the provider
func (s *pagerduty) SendEvent(ev *event.Event) error {
	logrus.Debugf("sending to pagerduty event: %v", ev)

	if len(s.integrationKey) == 0 {
		return errors.New("integration key is empty")
	}

	client := &http.Client{}

	reqBody := buildRequestBody(ev, s.integrationKey)
	buffer := bytes.NewBuffer([]byte(reqBody))

	request, err := http.NewRequest(http.MethodPost, pagerdutyAPIURL, buffer)
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
func (s *pagerduty) SendMessage(msg string) error {
	return nil
}

func buildRequestBody(ev *event.Event, key string) string {
	eventsText := "No events captured"
	logsText := "No logs captured"

	// add events part if it exists
	events := strings.TrimSpace(ev.Events)
	if len(events) > 0 {
		eventsText = ev.Events
	}

	// add logs part if it exists
	logs := strings.TrimSpace(ev.Logs)
	if len(logs) > 0 {
		logsText = ev.Logs
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
		key,
		fmt.Sprintf(defaultEventTitle, ev.Container),
		ev.Container,
		ev.Name,
		ev.Container,
		ev.Namespace,
		ev.Reason,
		eventsText,
		logsText)

	return reqBody
}
