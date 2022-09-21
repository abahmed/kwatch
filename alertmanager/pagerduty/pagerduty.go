package pagerduty

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"

	"github.com/abahmed/kwatch/event"
	"github.com/abahmed/kwatch/util"
	"github.com/sirupsen/logrus"
)

const (
	pagerdutyAPIURL   = "https://events.pagerduty.com/v2/enqueue"
	defaultEventTitle = "[%s] There is an issue with a container in a pod"
)

type Pagerduty struct {
	integrationKey string
}

// NewPagerDuty returns new PagerDuty instance
func NewPagerDuty(config map[string]string) *Pagerduty {
	integrationKey, ok := config["integrationKey"]
	if !ok || len(integrationKey) == 0 {
		logrus.Warnf("initializing pagerduty with an empty integration key")
		return nil
	}

	logrus.Infof("initializing pagerduty with the provided integration key")

	return &Pagerduty{
		integrationKey: integrationKey,
	}
}

// Name returns name of the provider
func (s *Pagerduty) Name() string {
	return "PagerDuty"
}

// SendEvent sends event to the provider
func (s *Pagerduty) SendEvent(ev *event.Event) error {
	logrus.Debugf("sending to pagerduty event: %v", ev)

	client := &http.Client{}

	reqBody := buildRequestBodyPagerDuty(ev, s.integrationKey)
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
func (s *Pagerduty) SendMessage(msg string) error {
	return nil
}

func buildRequestBodyPagerDuty(ev *event.Event, key string) string {
	eventsText := "No events captured"
	logsText := "No logs captured"

	// add events part if it exists
	events := strings.TrimSpace(ev.Events)
	if len(events) > 0 {
		eventsText = util.JsonEscape(ev.Events)
	}

	// add logs part if it exists
	logs := strings.TrimSpace(ev.Logs)
	if len(logs) > 0 {
		logsText = util.JsonEscape(ev.Logs)
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
