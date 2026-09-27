package incidentio

import (
	"context"
	"encoding/json"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
)

type incidentioPayload struct {
	Title            string                 `json:"title"`
	Description      string                 `json:"description,omitempty"`
	Status           string                 `json:"status"`
	DeduplicationKey string                 `json:"deduplication_key"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
}

// Incidentio sends events to an incident.io HTTP alert source.
type Incidentio struct {
	sender transport.Sender
	url    string
	apiKey string

	clusterName string
}

// NewIncidentio returns new Incident.io instance.
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

// Name returns name of the provider.
func (i *Incidentio) Name() string {
	return "Incident.io"
}

// UsesEventDelivery routes incidents through SendEvent, which carries the
// action and a stable key so incident.io can resolve the alert.
func (i *Incidentio) UsesEventDelivery() {}

// SendEvent fires or resolves one incident.io alert per kwatch incident,
// keyed by deduplication_key.
func (i *Incidentio) SendEvent(ctx context.Context, e *event.Event) error {
	status := "firing"
	if e.IsResolve() {
		status = "resolved"
	}
	payload := incidentioPayload{
		Title:            e.AlertTitle(250),
		Description:      e.AlertBody(i.clusterName),
		Status:           status,
		DeduplicationKey: e.AlertKey(),
		Metadata: map[string]interface{}{
			"cluster":   i.clusterName,
			"pod":       e.PodName,
			"container": e.ContainerName,
			"namespace": e.Namespace,
			"node":      e.NodeName,
			"reason":    e.Reason,
			"severity":  string(e.Severity),
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	headers := map[string]string{}
	if len(i.apiKey) > 0 {
		headers["Authorization"] = "Bearer " + i.apiKey
	}
	_, err = i.sender.Send(ctx, transport.Request{
		Provider: i.Name(), URL: i.url, Body: body,
		ContentType: "application/json", Headers: headers,
	})
	return err
}

// SendMessage sends a plain notice as one deduplicated alert.
func (i *Incidentio) SendMessage(ctx context.Context, msg string) error {
	return i.SendEvent(ctx, &event.Event{PodName: msg, Reason: "notify"})
}
