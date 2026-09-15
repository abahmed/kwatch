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
func (i *Ilert) SendEvent(ctx context.Context, e *event.Event) error {
	return i.SendMessage(ctx, e.FormatText(i.clusterName, ""))
}

// SendMessage sends text message to the provider
func (i *Ilert) SendMessage(ctx context.Context, msg string) error {
	payload := ilertPayload{
		EventType: "ALERT",
		Summary:   msg,
		Message:   msg,
		Priority:  i.priority,
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
