package pagerduty

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/util"
	"github.com/sirupsen/logrus"
)

const (
	pagerdutyAPIURL   = "https://events.pagerduty.com/v2/enqueue"
	defaultEventTitle = "[%s] There is an issue with a container in a pod"
)

type pagerdutyPayload struct {
	RoutingKey  string                  `json:"routing_key"`
	EventAction string                  `json:"event_action"`
	Payload     pagerdutyPayloadDetails `json:"payload"`
}

type pagerdutyPayloadDetails struct {
	Summary      string                 `json:"summary"`
	Source       string                 `json:"source"`
	Severity     string                 `json:"severity"`
	CustomDetail pagerdutyCustomDetails `json:"custom_details"`
}

type pagerdutyCustomDetails struct {
	Cluster   string `json:"Cluster"`
	Name      string `json:"Name"`
	Container string `json:"Container"`
	Namespace string `json:"Namespace"`
	Node      string `json:"Node"`
	Reason    string `json:"Reason"`
	Events    string `json:"Events"`
	Logs      string `json:"Logs"`
}

type Pagerduty struct {
	integrationKey string
	url            string

	// reference for general app configuration
	appCfg *config.App
}

// NewPagerDuty returns new PagerDuty instance
func NewPagerDuty(config map[string]interface{}, appCfg *config.App) *Pagerduty {
	integrationKey, ok := config["integrationKey"].(string)
	if !ok || len(integrationKey) == 0 {
		logrus.Warnf("initializing pagerduty with an empty integration key")
		return nil
	}

	logrus.Infof("initializing pagerduty with the provided integration key")

	return &Pagerduty{
		integrationKey: integrationKey,
		url:            pagerdutyAPIURL,
		appCfg:         appCfg,
	}
}

// Name returns name of the provider
func (s *Pagerduty) Name() string {
	return "PagerDuty"
}

// SendEvent sends event to the provider
func (s *Pagerduty) SendEvent(ev *event.Event) error {
	client := util.GetDefaultClient()

	reqBody, err := s.buildRequestBodyPagerDuty(ev, s.integrationKey)
	if err != nil {
		return err
	}
	buffer := bytes.NewBuffer([]byte(reqBody))

	request, err := http.NewRequest(http.MethodPost, s.url, buffer)
	if err != nil {
		return err
	}

	request.Header.Set("Content-Type", "application/json")

	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode > 202 {
		return fmt.Errorf(
			"call to teams alert returned status code %d",
			response.StatusCode)
	}

	return nil
}

// SendMessage sends text message to the provider
func (s *Pagerduty) SendMessage(msg string) error {
	return nil
}

func (s *Pagerduty) buildRequestBodyPagerDuty(
	ev *event.Event,
	key string) (string, error) {
	eventsText := "No events captured"
	logsText := "No logs captured"

	events := strings.TrimSpace(ev.Events)
	if len(events) > 0 {
		eventsText = ev.Events
	}

	logs := strings.TrimSpace(ev.Logs)
	if len(logs) > 0 {
		logsText = ev.Logs
	}

	payload := pagerdutyPayload{
		RoutingKey:  key,
		EventAction: "trigger",
		Payload: pagerdutyPayloadDetails{
			Summary:  fmt.Sprintf(defaultEventTitle, ev.ContainerName),
			Source:   ev.ContainerName,
			Severity: "critical",
			CustomDetail: pagerdutyCustomDetails{
				Cluster:   s.appCfg.ClusterName,
				Name:      ev.PodName,
				Container: ev.ContainerName,
				Namespace: ev.Namespace,
				Node:      ev.NodeName,
				Reason:    ev.Reason,
				Events:    eventsText,
				Logs:      logsText,
			},
		},
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	return string(bodyBytes), nil
}
