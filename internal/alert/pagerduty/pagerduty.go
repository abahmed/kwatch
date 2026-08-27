package pagerduty

import (
	"encoding/json"
	"fmt"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
)

const (
	pagerdutyAPIURL   = "https://events.pagerduty.com/v2/enqueue"
	defaultEventTitle = "[%s] There is an issue with a container in a pod"
)

type pagerdutyPayload struct {
	RoutingKey  string                  `json:"routing_key"`
	EventAction string                  `json:"event_action"`
	DedupKey    string                  `json:"dedup_key,omitempty"`
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
func NewPagerDuty(
	config map[string]interface{},
	appCfg *config.App,
) *Pagerduty {
	integrationKey, ok := config["integrationKey"].(string)
	if !ok || len(integrationKey) == 0 {
		klog.InfoS("initializing pagerduty with an empty integration key")
		return nil
	}

	klog.InfoS("initializing pagerduty with the provided integration key")

	return &Pagerduty{
		integrationKey: integrationKey,
		url:            pagerdutyAPIURL,
		appCfg:         appCfg,
	}
}

// Name returns name of the provider
func (p *Pagerduty) Name() string {
	return "PagerDuty"
}

func (p *Pagerduty) UsesEventDelivery() {}

// SendEvent sends event to the provider
func (p *Pagerduty) SendEvent(ev *event.Event) error {
	reqBody, err := p.buildRequestBodyPagerDuty(ev, p.integrationKey)
	if err != nil {
		return err
	}
	_, err = util.Send(
		util.Request{Provider: "PagerDuty", URL: p.url, Body: []byte(reqBody)},
	)
	return err
}

// SendMessage sends text message to the provider
func (p *Pagerduty) SendMessage(msg string) error {
	return nil
}

func (p *Pagerduty) buildRequestBodyPagerDuty(
	ev *event.Event,
	key string) (string, error) {
	eventAction := "trigger"
	if ev.Action == "resolved" {
		eventAction = "resolve"
	}

	summary := fmt.Sprintf("Alert: %s", util.OrDefault(ev.Reason, "unknown"))
	if ev.ContainerName != "" {
		summary = fmt.Sprintf(defaultEventTitle, ev.ContainerName)
	}

	source := util.OrDefault(
		ev.ContainerName,
		util.OrDefault(ev.PodName, "unknown"),
	)

	payload := pagerdutyPayload{
		RoutingKey:  key,
		EventAction: eventAction,
		DedupKey:    ev.DedupKey,
		Payload: pagerdutyPayloadDetails{
			Summary:  summary,
			Source:   source,
			Severity: "critical",
			CustomDetail: pagerdutyCustomDetails{
				Cluster:   p.appCfg.ClusterName,
				Name:      ev.PodName,
				Container: ev.ContainerName,
				Namespace: ev.Namespace,
				Node:      ev.NodeName,
				Reason:    ev.Reason,
				Events:    util.OrDefault(ev.Events, ""),
				Logs:      util.OrDefault(ev.Logs, ""),
			},
		},
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	return string(bodyBytes), nil
}
