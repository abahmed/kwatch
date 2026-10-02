package squadcast

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

const (
	squadcastAPIURL = "https://api.squadcast.com/v2/incidents/api/%s"
	// messageLimit bounds the incident message Squadcast shows in lists.
	messageLimit = 250
)

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

// SendIncident triggers, updates or resolves one Squadcast incident per
// kwatch incident, keyed by event_id.
func (s *Squadcast) SendIncident(
	ctx context.Context, m notification.Message,
) error {
	// A plain notice (startup, upgrade, test) or the startup summary is
	// not an incident. Sending it would page for problems that already
	// have their own alerts, so it is skipped.
	if m.IsInformational() {
		klog.V(4).InfoS("skipping informational message",
			"component", "delivery", "provider", s.Name())
		return nil
	}
	status := "trigger"
	if m.Resolved() {
		status = "resolve"
	}
	description := m.NoteText()
	if len(m.Output) > 0 {
		description += "\n\n" + strings.Join(m.Output, "\n")
	}
	payload := squadcastPayload{
		Message:     notification.Truncate(m.ShortText(), messageLimit),
		Description: description,
		Status:      status,
		EventID:     m.AlertKey(s.clusterName),
		Severity:    m.Route.Severity,
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

// SendMessage skips plain notices: on a paging service they would open an
// alert that nothing resolves.
func (s *Squadcast) SendMessage(ctx context.Context, msg string) error {
	return s.SendIncident(ctx, notification.Notice(msg))
}

// SkipsPlainMessages implements api.PlainMessageSkipper: plain messages
// become notices, which SendIncident skips.
func (s *Squadcast) SkipsPlainMessages() bool { return true }
