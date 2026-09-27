package alerta

import (
	"context"
	"encoding/json"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
)

const alertaAPIPath = "/api/alert"

type alertaPayload struct {
	Resource    string   `json:"resource"`
	Event       string   `json:"event"`
	Environment string   `json:"environment"`
	Severity    string   `json:"severity"`
	Service     []string `json:"service"`
	Text        string   `json:"text,omitempty"`
}

type Alerta struct {
	sender      transport.Sender
	url         string
	apiKey      string
	environment string
	service     string

	clusterName string
}

// NewAlerta returns a new Alerta object

func NewAlerta(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Alerta {
	url, ok := config["url"].(string)
	if !ok || len(url) == 0 {
		klog.InfoS("initializing alerta with empty url")
		return nil
	}

	apiKey, ok := config["apiKey"].(string)
	if !ok || len(apiKey) == 0 {
		klog.InfoS("initializing alerta with empty apiKey")
		return nil
	}

	environment, _ := config["environment"].(string)
	if len(environment) == 0 {
		environment = "Production"
	}

	service, _ := config["service"].(string)
	if len(service) == 0 {
		service = "kwatch"
	}

	klog.InfoS("initializing alerta", "url", url, "environment", environment)

	return &Alerta{
		sender:      transport.NewSender(dependencies),
		url:         strings.TrimRight(url, "/") + alertaAPIPath,
		apiKey:      apiKey,
		environment: environment,
		service:     service,
		clusterName: clusterName,
	}
}

// Name returns name of the provider
func (s *Alerta) Name() string {
	return "Alerta"
}

// SendEvent sends event to the provider
// UsesEventDelivery routes incidents through SendEvent, which carries the
// action and a stable key so Alerta can close the alert.
func (s *Alerta) UsesEventDelivery() {}

// SendEvent raises or closes one Alerta alert per kwatch incident. Alerta
// deduplicates on environment, resource and event, so the resource carries
// the incident key.
func (s *Alerta) SendEvent(ctx context.Context, e *event.Event) error {
	resource := e.AlertKey()
	if len(s.clusterName) > 0 {
		resource = s.clusterName + "/" + resource
	}
	severity := "critical"
	switch {
	case e.IsResolve():
		severity = "normal"
	case e.IsNotice():
		severity = "informational"
	}
	eventName := e.Reason
	if eventName == "" || e.IsNotice() {
		eventName = "kwatch"
	}
	payload := alertaPayload{
		Resource:    resource,
		Event:       eventName,
		Environment: s.environment,
		Severity:    severity,
		Service:     []string{s.service},
		Text:        e.AlertBody(s.clusterName),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = s.sender.Send(ctx, transport.Request{
		Provider: s.Name(), URL: s.url, Body: body,
		ContentType: "application/json", Headers: map[string]string{
			"Authorization": "Key " + s.apiKey,
		},
	})
	return err
}

// SendMessage sends a plain notice as an informational alert.
func (s *Alerta) SendMessage(ctx context.Context, msg string) error {
	return s.SendEvent(ctx, &event.Event{PodName: msg, Reason: "notify"})
}
