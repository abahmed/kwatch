package pagerduty

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/model"
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
	sender         transport.Sender
	integrationKey string
	url            string

	// reference for general app configuration
	clusterName string
}

// NewPagerDuty returns new PagerDuty instance

func NewPagerDuty(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Pagerduty {
	integrationKey, ok := config["integrationKey"].(string)
	if !ok || len(integrationKey) == 0 {
		klog.InfoS("initializing pagerduty with an empty integration key")
		return nil
	}

	klog.InfoS("initializing pagerduty with the provided integration key")

	return &Pagerduty{
		sender:         transport.NewSender(dependencies),
		integrationKey: integrationKey,
		url:            pagerdutyAPIURL,
		clusterName:    clusterName,
	}
}

// Name returns name of the provider
func (p *Pagerduty) Name() string {
	return "PagerDuty"
}

func (p *Pagerduty) UsesEventDelivery() {}

// SendEvent sends event to the provider
func (p *Pagerduty) SendEvent(ctx context.Context, ev *event.Event) error {
	reqBody, err := p.buildRequestBodyPagerDuty(ev, p.integrationKey)
	if err != nil {
		return err
	}
	_, err = p.sender.Send(ctx, transport.Request{
		Provider: "PagerDuty", URL: p.url, Body: []byte(reqBody),
	})
	return err
}

// SendMessage sends text message to the provider
func (p *Pagerduty) SendMessage(ctx context.Context, msg string) error {
	return nil
}

// pagerdutySeverity maps kwatch's severity onto the four PagerDuty accepts.
//
// Every alert used to be sent as "critical", which is what escalation
// policies page on: a warning about CPU throttling woke somebody at 3am with
// the same urgency as a cluster-wide outage, and teams responded by muting
// the integration. An unknown or unset severity is "error", not "critical" --
// the safe default is the one that files rather than pages.
func pagerdutySeverity(sev model.Severity) string {
	switch sev {
	case model.SeverityCritical:
		return "critical"
	case model.SeverityHigh:
		return "error"
	case model.SeverityMedium, model.SeverityWarning:
		return "warning"
	case model.SeverityNormal:
		return "info"
	}
	return "error"
}

func (p *Pagerduty) buildRequestBodyPagerDuty(
	ev *event.Event,
	key string) (string, error) {
	eventAction := "trigger"
	if ev.Action == "resolved" {
		eventAction = "resolve"
	}

	summary := fmt.Sprintf("Alert: %s", format.OrDefault(ev.Reason, "unknown"))
	if narrative := strings.TrimSpace(ev.Narrative); narrative != "" {
		summary = strings.SplitN(narrative, "\n", 2)[0]
	}
	if ev.Narrative == "" && ev.ContainerName != "" {
		summary = fmt.Sprintf(defaultEventTitle, ev.ContainerName)
	}

	source := format.OrDefault(
		ev.ContainerName,
		format.OrDefault(ev.PodName, "unknown"),
	)

	payload := pagerdutyPayload{
		RoutingKey:  key,
		EventAction: eventAction,
		DedupKey:    ev.DedupKey,
		Payload: pagerdutyPayloadDetails{
			Summary:  summary,
			Source:   source,
			Severity: pagerdutySeverity(ev.Severity),
			CustomDetail: pagerdutyCustomDetails{
				Cluster:   p.clusterName,
				Name:      ev.PodName,
				Container: ev.ContainerName,
				Namespace: ev.Namespace,
				Node:      ev.NodeName,
				Reason:    ev.Reason,
				Events:    "",
				Logs:      format.OrDefault(ev.Narrative, ev.Logs),
			},
		},
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	return string(bodyBytes), nil
}
