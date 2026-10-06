package ilert

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/safetext"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

const (
	ilertAPIURL = "https://api.ilert.com/api/v1/events/push/%s"
	// summaryLimit bounds the alert summary iLert shows in lists.
	summaryLimit = 250
)

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

// SendIncident raises, updates or resolves one iLert alert per kwatch
// incident, keyed by alertKey.
func (i *Ilert) SendIncident(
	ctx context.Context, m notification.Message,
) error {
	// A plain notice (startup, upgrade, test) or the startup summary is
	// not an incident. Sending it would page for problems that already
	// have their own alerts, so it is skipped.
	if m.IsInformational() {
		klog.V(4).InfoS("skipping informational message",
			"component", "delivery", "provider", i.Name())
		return nil
	}
	eventType := "ALERT"
	if m.Resolved() {
		eventType = "RESOLVE"
	}
	details := safetext.PlainWithOutput(
		m.NoteText(), m.Output, "\n\n", safetext.DetailsLimit)
	payload := ilertPayload{
		EventType: eventType,
		Summary:   notification.Truncate(m.ShortText(), summaryLimit),
		Message:   m.NoteText(),
		Details:   details,
		Priority:  i.priorityFor(m),
		AlertKey:  m.AlertKey(i.clusterName),
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

// priorityFor keeps the configured priority for real incidents;
// informational incidents are LOW so they never page.
func (i *Ilert) priorityFor(m notification.Message) string {
	if m.Route.Severity == "info" {
		return "LOW"
	}
	return i.priority
}

// SendMessage skips plain notices: on a paging service they would open an
// alert that nothing resolves.
func (i *Ilert) SendMessage(ctx context.Context, msg string) error {
	return i.SendIncident(ctx, notification.Notice(msg))
}

// SkipsPlainMessages implements api.PlainMessageSkipper: plain messages
// become notices, which SendIncident skips.
func (i *Ilert) SkipsPlainMessages() bool { return true }
