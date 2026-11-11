package ilert

import (
	"context"
	"encoding/json"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/safetext"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

const (
	// ilertAPIURL is the documented Events API endpoint. The integration key
	// travels in the request body, never in the URL.
	ilertAPIURL = "https://api.ilert.com/api/events"
	// summaryLimit bounds the alert summary iLert shows in lists.
	summaryLimit = 250
)

// ilertPayload holds only fields the Events API documents.
type ilertPayload struct {
	IntegrationKey string `json:"integrationKey"`
	EventType      string `json:"eventType"`
	Summary        string `json:"summary"`
	Priority       string `json:"priority,omitempty"`
	Details        string `json:"details,omitempty"`
	AlertKey       string `json:"alertKey,omitempty"`
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

	priority := normalizePriority(config["priority"])

	klog.InfoS("initializing ilert", "priority", priority)

	return &Ilert{
		sender:         transport.NewSender(dependencies),
		url:            ilertAPIURL,
		integrationKey: integrationKey,
		priority:       priority,
		clusterName:    clusterName,
	}
}

// normalizePriority maps the configured priority to one the Events API
// accepts (HIGH or LOW). CRITICAL stays valid in configs and is sent as
// HIGH; an unknown value falls back to HIGH.
func normalizePriority(v interface{}) string {
	p, ok := v.(string)
	if !ok || len(p) == 0 {
		return "HIGH"
	}
	switch strings.ToUpper(strings.TrimSpace(p)) {
	case "HIGH", "CRITICAL":
		return "HIGH"
	case "LOW":
		return "LOW"
	}
	klog.InfoS("unknown ilert priority, using HIGH", "priority", p)
	return "HIGH"
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
	payload := i.buildPayload(m)
	if !m.PageKeyed && !m.RaisesPage {
		return i.send(ctx, payload)
	}
	paged := payload
	paged.AlertKey = m.AlertKeyVariant(
		i.clusterName, notification.PageKeySuffix, 0)
	if m.RaisesPage {
		paged.Priority = i.priority
	}
	if !m.Resolved() && !m.RaisesPage {
		return i.send(ctx, paged)
	}
	// The raise opens the page key first, then closes the alert opened
	// below the page tier: an open alert never changes its priority, so
	// only the new alert pages. The resolve closes both keys.
	if m.RaisesPage {
		if err := i.send(ctx, paged); err != nil {
			return err
		}
		return i.closeBase(ctx, closing(payload))
	}
	if err := i.closeBase(ctx, payload); err != nil {
		return err
	}
	return i.send(ctx, paged)
}

// closeBase sends the resolve of the base key that goes with the page
// key. A pager that never heard the incident below the page tier has no
// alert under it, so an answer that only says so is no failure.
func (i *Ilert) closeBase(ctx context.Context, payload ilertPayload) error {
	if err := i.send(ctx, payload); err != nil && !closedAlready(err) {
		return err
	}
	return nil
}

func (i *Ilert) buildPayload(m notification.Message) ilertPayload {
	eventType := "ALERT"
	if m.Resolved() {
		eventType = "RESOLVE"
	}
	details := safetext.PlainWithOutput(
		m.NoteText(), m.Output, "\n\n", safetext.DetailsLimit)
	return ilertPayload{
		IntegrationKey: i.integrationKey,
		EventType:      eventType,
		Summary:        notification.Truncate(m.ShortText(), summaryLimit),
		Details:        details,
		Priority:       i.priorityFor(m),
		AlertKey:       m.AlertKey(i.clusterName),
	}
}

func (i *Ilert) send(ctx context.Context, payload ilertPayload) error {
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

// closing is the RESOLVE of the alert payload names.
func closing(payload ilertPayload) ilertPayload {
	payload.EventType, payload.Priority = "RESOLVE", ""
	return payload
}

// closedAlready reports a refused resolve of the base key that only says
// there was nothing open to close. The Events API documents 202 for an
// accepted event (it logs "no open alert with key" and changes nothing)
// and 400 for a bad request; a resolve of a key it never saw is not
// documented as an error, so a 400, 404 or 422 on that one resolve counts
// as done: the alert is not open, which is what the resolve asked for.
func closedAlready(err error) bool {
	code, ok := transport.StatusOf(err)
	return ok && (code == 400 || code == 404 || code == 422)
}

// priorityFor follows the incident severity: only a page-tier incident
// (critical) gets the configured priority, the one that pages. A notify or
// low tier incident (warning, info) is LOW, so it never wakes anyone. A
// message with no route severity is judged by its status.
func (i *Ilert) priorityFor(m notification.Message) string {
	switch m.Route.Severity {
	case "critical":
		return i.priority
	case "":
		if m.Status == notification.StatusCritical {
			return i.priority
		}
	}
	return "LOW"
}

// SendMessage skips plain notices: on a paging service they would open an
// alert that nothing resolves.
func (i *Ilert) SendMessage(ctx context.Context, msg string) error {
	return i.SendIncident(ctx, notification.Notice(msg))
}

// SkipsPlainMessages implements api.PlainMessageSkipper: plain messages
// become notices, which SendIncident skips.
func (i *Ilert) SkipsPlainMessages() bool { return true }
