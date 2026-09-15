package incidentio

import (
	"context"
	"encoding/json"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
)

type incidentioPayload struct {
	EventType string                 `json:"event_type"`
	Source    string                 `json:"source"`
	Severity  string                 `json:"severity,omitempty"`
	Message   string                 `json:"message"`
	Payload   map[string]interface{} `json:"payload,omitempty"`
}

type Incidentio struct {
	sender transport.Sender
	url    string
	apiKey string

	clusterName string
}

// NewIncidentio returns a new Incidentio object

func NewIncidentio(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Incidentio {
	url, ok := config["url"].(string)
	if !ok || len(url) == 0 {
		klog.InfoS("initializing incidentio with empty url")
		return nil
	}

	apiKey, _ := config["apiKey"].(string)

	klog.InfoS("initializing incidentio")

	return &Incidentio{
		sender:      transport.NewSender(dependencies),
		url:         url,
		apiKey:      apiKey,
		clusterName: clusterName,
	}
}

// Name returns name of the provider
func (i *Incidentio) Name() string {
	return "Incident.io"
}

// SendEvent sends event to the provider
func (i *Incidentio) SendEvent(ctx context.Context, e *event.Event) error {
	payload := incidentioPayload{
		EventType: "kwatch.incident",
		Source:    "kwatch",
		Severity:  string(e.Severity),
		Message:   e.FormatText(i.clusterName, ""),
		Payload: map[string]interface{}{
			"pod":       e.PodName,
			"container": e.ContainerName,
			"namespace": e.Namespace,
			"node":      e.NodeName,
			"reason":    e.Reason,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	return i.send(ctx, body)
}

// SendMessage sends text message to the provider
func (i *Incidentio) SendMessage(ctx context.Context, msg string) error {
	payload := incidentioPayload{
		EventType: "kwatch.incident",
		Source:    "kwatch",
		Message:   msg,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	return i.send(ctx, body)
}

func (i *Incidentio) send(ctx context.Context, body []byte) error {
	headers := map[string]string{}
	if len(i.apiKey) > 0 {
		headers["Authorization"] = "Bearer " + i.apiKey
	}

	_, err := i.sender.Send(ctx, transport.Request{
		Provider: i.Name(), URL: i.url, Body: body,
		ContentType: "application/json", Headers: headers,
	})
	return err
}
