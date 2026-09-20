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
func (s *Squadcast) SendEvent(ctx context.Context, e *event.Event) error {
	payload := squadcastPayload{
		Message:     e.Reason,
		Description: e.FormatText(s.clusterName, ""),
		Status:      "trigger",
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

// SendMessage sends text message to the provider
func (s *Squadcast) SendMessage(ctx context.Context, msg string) error {
	payload := squadcastPayload{
		Message:     "kwatch alert",
		Description: msg,
		Status:      "trigger",
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
