package ilert

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
)

const ilertAPIURL = "https://api.ilert.com/api/v1/events/push/%s"

type ilertPayload struct {
	EventType string `json:"eventType"`
	Summary   string `json:"summary"`
	Message   string `json:"message"`
	Priority  string `json:"priority,omitempty"`
	Details   string `json:"details,omitempty"`
	AlertKey  string `json:"alertKey,omitempty"`
}

type Ilert struct {
	sender         transport.Sender
	url            string
	integrationKey string
	priority       string

	clusterName string
}

// NewIlert returns a new Ilert object

func NewIlert(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Ilert {
	integrationKey, ok := config["integrationKey"].(string)
	if !ok || len(integrationKey) == 0 {
		klog.InfoS("initializing ilert with empty integrationKey")
		return nil
	}

	priority := "HIGH"
	if p, ok := config["priority"].(string); ok && len(p) > 0 {
		priority = p
	}

	klog.InfoS("initializing ilert", "priority", priority)

	return &Ilert{
		sender:         transport.NewSender(dependencies),
		url:            fmt.Sprintf(ilertAPIURL, integrationKey),
		integrationKey: integrationKey,
		priority:       priority,
		clusterName:    clusterName,
	}
}

// Name returns name of the provider
func (i *Ilert) Name() string {
	return "Ilert"
}

// SendEvent sends event to the provider
// UsesEventDelivery routes incidents through SendEvent, which carries the
// action and a stable key so iLert can resolve the alert.
func (i *Ilert) UsesEventDelivery() {}

// SendEvent raises or resolves one iLert alert per kwatch incident, keyed by
// alertKey.
func (i *Ilert) SendEvent(ctx context.Context, e *event.Event) error {
	eventType := "ALERT"
	if e.IsResolve() {
		eventType = "RESOLVE"
	}
	priority := i.priority
	if e.IsNotice() {
		priority = "LOW"
	}
	payload := ilertPayload{
		EventType: eventType,
		Summary:   e.AlertTitle(250),
		Message:   e.AlertBody(i.clusterName),
		Details:   e.AlertBody(i.clusterName),
		Priority:  priority,
		AlertKey:  e.AlertKey(),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = i.sender.Send(ctx, transport.Request{
		Provider: i.Name(), URL: i.url, Body: body,
		ContentType: "application/json",
	})
	return err
}

// SendMessage sends a plain notice as one low-priority deduplicated alert.
func (i *Ilert) SendMessage(ctx context.Context, msg string) error {
	return i.SendEvent(ctx, &event.Event{PodName: msg, Reason: "notify"})
}
