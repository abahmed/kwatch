package goalert

import (
	"context"
	"encoding/json"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
)

const (
	goalertAPIURL  = "https://goalert.example.com"
	goalertAPIPath = "/api/v2/events"
)

type goalertPayload struct {
	Type    string `json:"type"`
	Service string `json:"serviceID,omitempty"`
	Message string `json:"message,omitempty"`
}

type Goalert struct {
	sender    transport.Sender
	url       string
	token     string
	serviceID string

	clusterName string
}

// NewGoalert returns a new Goalert object

func NewGoalert(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Goalert {
	token, ok := config["token"].(string)
	if !ok || len(token) == 0 {
		klog.InfoS("initializing goalert with empty token")
		return nil
	}

	serviceID, ok := config["serviceId"].(string)
	if !ok || len(serviceID) == 0 {
		klog.InfoS("initializing goalert with empty serviceId")
		return nil
	}

	server := goalertAPIURL
	if u, ok := config["url"].(string); ok && len(u) > 0 {
		server = u
	}

	klog.InfoS("initializing goalert", "url", server, "serviceID", serviceID)

	return &Goalert{
		sender:      transport.NewSender(dependencies),
		url:         strings.TrimRight(server, "/") + goalertAPIPath,
		token:       token,
		serviceID:   serviceID,
		clusterName: clusterName,
	}
}

// Name returns name of the provider
func (s *Goalert) Name() string {
	return "GoAlert"
}

// SendEvent sends event to the provider
func (s *Goalert) SendEvent(ctx context.Context, e *event.Event) error {
	msg := e.FormatText(s.clusterName, "")
	return s.SendMessage(ctx, msg)
}

// SendMessage sends text message to the provider
func (s *Goalert) SendMessage(ctx context.Context, msg string) error {
	payload := goalertPayload{
		Type:    "incident.create",
		Service: s.serviceID,
		Message: msg,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = s.sender.Send(ctx, transport.Request{
		Provider: s.Name(), URL: s.url, Body: body,
		ContentType: "application/json", Headers: map[string]string{
			"Authorization": "Bearer " + s.token,
		},
	})
	return err
}
