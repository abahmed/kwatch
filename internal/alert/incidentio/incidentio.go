package incidentio

import (
	"context"
	"encoding/json"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/safetext"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

// titleLimit bounds the alert title incident.io shows in lists.
const titleLimit = 250

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

	if !transport.ValidEndpoint(url) {
		klog.InfoS("initializing incidentio with an invalid url",
			"setting", "url")
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

// SendIncident fires, updates or resolves one incident.io alert per kwatch
// incident, keyed by deduplication_key.
func (i *Incidentio) SendIncident(
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
	status := "firing"
	if m.Resolved() {
		status = "resolved"
	}
	description := safetext.PlainWithOutput(
		m.NoteText(), m.Output, "\n\n", safetext.DetailsLimit)
	payload := incidentioPayload{
		Title:            notification.Truncate(m.ShortText(), titleLimit),
		Description:      description,
		Status:           status,
		DeduplicationKey: m.AlertKey(i.clusterName),
		Metadata: map[string]interface{}{
			"cluster":    i.clusterName,
			"namespaces": m.Route.Namespaces,
			"reasons":    m.Route.Reasons,
			"severity":   m.Route.Severity,
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

// SendMessage skips plain notices: on a paging service they would open an
// alert that nothing resolves.
func (i *Incidentio) SendMessage(ctx context.Context, msg string) error {
	return i.SendIncident(ctx, notification.Notice(msg))
}

// SkipsPlainMessages implements api.PlainMessageSkipper: plain messages
// become notices, which SendIncident skips.
func (i *Incidentio) SkipsPlainMessages() bool { return true }
