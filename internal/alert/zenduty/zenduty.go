package zenduty

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

const zendutyAPIURL = "https://www.zenduty.com/api/events"

var alertTypes = []string{
	"critical",
	"acknowledged",
	"resolved",
	"error",
	"warning",
	"info",
}

type Zenduty struct {
	sender         transport.Sender
	integrationkey string
	url            string
	alertType      string

	// reference for general app configuration
	clusterName string
}

type zendutyPayload struct {
	Message   string `json:"message"`
	Summary   string `json:"summary"`
	AlertType string `json:"alert_type"`
	EntityID  string `json:"entity_id,omitempty"`
}

// NewZenduty returns new zenduty instance

func NewZenduty(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Zenduty {
	integrationKey, ok := config["integrationKey"].(string)
	if !ok || len(integrationKey) == 0 {
		klog.InfoS("initializing zenduty with empty webhook url")
		return nil
	}

	klog.InfoS("initializing zenduty with secret apikey")

	// An alertType that is absent or invalid leaves the field empty, and
	// each alert then carries its own severity. Defaulting everything to
	// "critical" meant a CPU-throttling warning arrived at the level whose
	// job is to page, which is how integrations end up muted.
	alertType, ok := config["alertType"].(string)
	if !ok || !slices.Contains(alertTypes, alertType) {
		alertType = ""
	}

	return &Zenduty{
		sender:         transport.NewSender(dependencies),
		integrationkey: integrationKey,
		url:            zendutyAPIURL,
		alertType:      alertType,
		clusterName:    clusterName,
	}
}

// Name returns name of the provider
func (z *Zenduty) Name() string {
	return "Zenduty"
}

// SendMessage skips plain notices: on a paging service they would open an
// alert that nothing resolves.
func (z *Zenduty) SendMessage(ctx context.Context, msg string) error {
	return z.SendIncident(ctx, notification.Notice(msg))
}

// SkipsPlainMessages implements api.PlainMessageSkipper: plain messages
// become notices, which SendIncident skips.
func (z *Zenduty) SkipsPlainMessages() bool { return true }

// SendIncident opens, updates or resolves one Zenduty alert per incident.
// The alert's entity_id is the incident's stable alert key.
func (z *Zenduty) SendIncident(
	ctx context.Context, m notification.Message,
) error {
	// A plain notice (startup, upgrade, test) or the startup summary is
	// not an incident. Sending it would page for problems that already
	// have their own alerts, so it is skipped.
	if m.IsInformational() {
		klog.V(4).InfoS("skipping informational message",
			"component", "delivery", "provider", z.Name())
		return nil
	}
	body, err := json.Marshal(z.buildPayload(m))
	if err != nil {
		return fmt.Errorf("failed to marshal zenduty payload: %w", err)
	}
	return z.sendAPI(ctx, body)
}

// sendAPI sends http request to Zenduty API
func (z *Zenduty) sendAPI(
	ctx context.Context,
	content []byte,
) error {
	_, err := z.sender.Send(ctx, transport.Request{
		Provider: "Zenduty",
		URL:      z.url + "/" + z.integrationkey + "/",
		Body:     content,
	})
	return err
}

// alertTypeFor is "resolved" for a resolved incident, the operator's
// configured alert type when they set one, and the incident's own
// severity otherwise.
func (z *Zenduty) alertTypeFor(m notification.Message) string {
	if m.Resolved() {
		return "resolved"
	}
	if z.alertType != "" {
		return z.alertType
	}
	switch m.Route.Severity {
	case "critical":
		return "critical"
	case "warning":
		return "warning"
	case "info":
		return "info"
	}
	if m.Status == notification.StatusCritical {
		return "critical"
	}
	return "warning"
}

// maxMessageBytes is Zenduty's limit for the alert message (title).
const maxMessageBytes = 130

func (z *Zenduty) buildPayload(m notification.Message) zendutyPayload {
	summary := m.NoteText()
	if len(m.Output) > 0 {
		summary += "\n\nLast output:\n" + strings.Join(m.Output, "\n")
	}
	if z.clusterName != "" {
		summary += "\n\nCluster: " + z.clusterName
	}
	return zendutyPayload{
		Message:   notification.Truncate(m.ShortText(), maxMessageBytes),
		Summary:   summary,
		AlertType: z.alertTypeFor(m),
		EntityID:  m.AlertKey(z.clusterName),
	}
}
