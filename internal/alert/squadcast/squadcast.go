package squadcast

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
)

const squadcastAPIURL = "https://api.squadcast.com/v2/incidents/api/%s"

type squadcastPayload struct {
	Message     string `json:"message"`
	Description string `json:"description"`
	Status      string `json:"status"`
	EventID     string `json:"event_id,omitempty"`
	Severity    string `json:"severity,omitempty"`
}

type Squadcast struct {
	sender     transport.Sender
	url        string
	serviceKey string

	clusterName string
}

// NewSquadcast returns a new Squadcast object

func NewSquadcast(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Squadcast {
	serviceKey, ok := config["serviceKey"].(string)
	if !ok || len(serviceKey) == 0 {
		klog.InfoS("initializing squadcast with empty serviceKey")
		return nil
	}

	klog.InfoS("initializing squadcast")

	return &Squadcast{
		sender:      transport.NewSender(dependencies),
		url:         fmt.Sprintf(squadcastAPIURL, serviceKey),
		serviceKey:  serviceKey,
		clusterName: clusterName,
	}
}

// Name returns name of the provider
func (s *Squadcast) Name() string {
	return "Squadcast"
}

// SendEvent sends event to the provider
// UsesEventDelivery routes problems through SendEvent, which carries the
// action and a stable key so Squadcast can deduplicate and resolve.
func (s *Squadcast) UsesEventDelivery() {}

// SendEvent triggers, updates or resolves one Squadcast incident per kwatch
// problem, keyed by event_id.
func (s *Squadcast) SendEvent(ctx context.Context, e *event.Event) error {
	status := "trigger"
	if e.IsResolve() {
		status = "resolve"
	}
	payload := squadcastPayload{
		Message:     e.AlertTitle(250),
		Description: e.AlertBody(s.clusterName),
		Status:      status,
		EventID:     e.AlertKey(),
		Severity:    string(e.Severity),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = s.sender.Send(ctx, transport.Request{
		Provider: s.Name(), URL: s.url, Body: body,
		ContentType: "application/json",
	})
	return err
}

// SendMessage sends a plain notice as one deduplicated Squadcast alert.
func (s *Squadcast) SendMessage(ctx context.Context, msg string) error {
	return s.SendEvent(ctx, &event.Event{PodName: msg, Reason: "notify"})
}
