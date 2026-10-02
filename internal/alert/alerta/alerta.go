package alerta

import (
	"context"
	"encoding/json"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

const alertaAPIPath = "/api/alert"

type alertaPayload struct {
	Resource    string   `json:"resource"`
	Event       string   `json:"event"`
	Environment string   `json:"environment"`
	Severity    string   `json:"severity"`
	Service     []string `json:"service"`
	Status      string   `json:"status,omitempty"`
	Value       string   `json:"value,omitempty"`
	Text        string   `json:"text,omitempty"`

	Attributes map[string]string `json:"attributes,omitempty"`
}

// incidentEvent is constant so an update or resolve whose reason changed
// still matches the alert Alerta deduplicates on (environment, resource,
// event). The reason travels as an attribute instead.
const incidentEvent = "incident"

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

	if !transport.ValidEndpoint(url) {
		klog.InfoS("initializing alerta with an invalid url",
			"setting", "url")
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

	klog.InfoS("initializing alerta",
		"url", transport.LogURL(url),
		"environment", environment)

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

// SendIncident raises or closes one Alerta alert per kwatch incident.
// Alerta deduplicates on environment, resource and event, so the resource
// carries the incident's alert key.
func (s *Alerta) SendIncident(
	ctx context.Context, m notification.Message,
) error {
	body, err := json.Marshal(s.buildPayload(m))
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

func (s *Alerta) buildPayload(m notification.Message) alertaPayload {
	// The resource is the stable dedup key, so an alert opened before a
	// state reset is still the one a later resolve closes.
	resource := m.AlertKey("")
	if len(s.clusterName) > 0 {
		resource = s.clusterName + "/" + resource
	}
	eventName := incidentEvent
	if m.IsNotice() {
		eventName = "kwatch"
	}
	text := m.NoteText()
	if len(m.Output) > 0 {
		text += "\n\nLast output:\n" + strings.Join(m.Output, "\n")
	}
	payload := alertaPayload{
		Resource:    resource,
		Event:       eventName,
		Environment: s.environment,
		Severity:    alertaSeverity(m),
		Service:     []string{s.service},
		Value:       m.ShortText(),
		Text:        text,
		Attributes:  map[string]string{"summary": m.ShortText()},
	}
	if m.Resolved() {
		payload.Status = "closed"
	}
	if len(m.Route.Reasons) > 0 {
		payload.Attributes["reason"] = strings.Join(m.Route.Reasons, ",")
	}
	return payload
}

// alertaSeverity maps the incident onto Alerta's severities. A resolve is
// "normal", which closes the alert.
func alertaSeverity(m notification.Message) string {
	switch {
	case m.Resolved():
		return "normal"
	case m.IsNotice():
		return "informational"
	}
	switch m.Route.Severity {
	case "warning":
		return "warning"
	case "info":
		return "informational"
	}
	if m.Route.Severity == "" && m.Status != notification.StatusCritical {
		return "warning"
	}
	return "critical"
}

// SendMessage sends a plain notice as an informational alert.
func (s *Alerta) SendMessage(ctx context.Context, msg string) error {
	return s.SendIncident(ctx, notification.Notice(msg))
}
