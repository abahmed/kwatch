package pagerduty

import (
	"context"
	"encoding/json"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

const (
	pagerdutyAPIURL = "https://events.pagerduty.com/v2/enqueue"
	// summaryLimit is the Events API v2 maximum summary length.
	summaryLimit = 1024
)

type pagerdutyPayload struct {
	RoutingKey  string                  `json:"routing_key"`
	EventAction string                  `json:"event_action"`
	DedupKey    string                  `json:"dedup_key"`
	Payload     *pagerdutyPayloadDetail `json:"payload,omitempty"`
}

type pagerdutyPayloadDetail struct {
	Summary      string                 `json:"summary"`
	Source       string                 `json:"source"`
	Severity     string                 `json:"severity"`
	CustomDetail pagerdutyCustomDetails `json:"custom_details"`
}

type pagerdutyCustomDetails struct {
	Cluster    string   `json:"cluster,omitempty"`
	Details    string   `json:"details"`
	Output     []string `json:"output,omitempty"`
	Namespaces []string `json:"namespaces,omitempty"`
	Reasons    []string `json:"reasons,omitempty"`
}

// Pagerduty triggers one PagerDuty alert per incident through the Events
// API v2 and resolves it with the same dedup key.
type Pagerduty struct {
	sender         transport.Sender
	integrationKey string
	url            string

	clusterName string
}

// NewPagerDuty returns new PagerDuty instance
func NewPagerDuty(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Pagerduty {
	integrationKey, ok := config["integrationKey"].(string)
	if !ok || len(integrationKey) == 0 {
		klog.InfoS("initializing pagerduty with an empty integration key")
		return nil
	}

	klog.InfoS("initializing pagerduty with the provided integration key")

	return &Pagerduty{
		sender:         transport.NewSender(dependencies),
		integrationKey: integrationKey,
		url:            pagerdutyAPIURL,
		clusterName:    clusterName,
	}
}

// Name returns name of the provider
func (p *Pagerduty) Name() string {
	return "PagerDuty"
}

// SendMessage skips plain notices: on a paging service they would open an
// alert that nothing resolves.
func (p *Pagerduty) SendMessage(ctx context.Context, msg string) error {
	return p.SendIncident(ctx, notification.Notice(msg))
}

// SkipsPlainMessages implements api.PlainMessageSkipper: plain messages
// become notices, which SendIncident skips.
func (p *Pagerduty) SkipsPlainMessages() bool { return true }

// SendIncident triggers (or updates) the incident's alert, or resolves it.
func (p *Pagerduty) SendIncident(
	ctx context.Context, m notification.Message,
) error {
	// A plain notice (startup, upgrade, test) or the startup summary is
	// not an incident. Sending it would page for problems that already
	// have their own alerts, so it is skipped.
	if m.IsInformational() {
		klog.V(4).InfoS("skipping informational message",
			"component", "delivery", "provider", p.Name())
		return nil
	}
	for _, payload := range p.payloads(m) {
		if err := p.send(ctx, payload); err != nil {
			return err
		}
	}
	return nil
}

func (p *Pagerduty) send(ctx context.Context, payload pagerdutyPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = p.sender.Send(ctx, transport.Request{
		Provider: "PagerDuty", URL: p.url, Body: body,
	})
	return err
}

// payloads are the events one message sends. A message that raises the
// page (RaisesPage) triggers the page key at critical first and then
// resolves the alert opened below the page tier: the incident that alert
// opened keeps its low urgency, so only a new alert pages. Later messages
// go to the page key, and the resolve closes both keys. Each event is
// idempotent, so a retry of the whole message is safe.
func (p *Pagerduty) payloads(m notification.Message) []pagerdutyPayload {
	base := p.buildPayload(m)
	if !m.PageKeyed && !m.RaisesPage {
		return []pagerdutyPayload{base}
	}
	paged := base
	paged.DedupKey = m.AlertKeyVariant(
		p.clusterName, notification.PageKeySuffix, maxDedupKey)
	if m.Resolved() {
		return []pagerdutyPayload{base, paged}
	}
	if !m.RaisesPage {
		return []pagerdutyPayload{paged}
	}
	detail := *paged.Payload
	detail.Severity = "critical"
	paged.Payload = &detail
	closeBase := pagerdutyPayload{RoutingKey: p.integrationKey,
		EventAction: "resolve", DedupKey: base.DedupKey}
	return []pagerdutyPayload{paged, closeBase}
}

// pagerdutySeverity maps the incident onto the four severities PagerDuty
// accepts. Only critical incidents page as "critical"; an unknown
// severity is "warning", the safe default that files rather than pages.
func pagerdutySeverity(m notification.Message) string {
	switch {
	case m.Route.Severity == "critical":
		return "critical"
	case m.Route.Severity == "info":
		return "info"
	case m.Route.Severity == "warning":
		return "warning"
	case m.Status == notification.StatusCritical:
		return "critical"
	}
	return "warning"
}

// detailsLimit caps the note and the output, each in bytes, so the event
// stays far under the Events API v2 limit of 512 KB even after JSON
// escaping.
const detailsLimit = 64000

// capLines keeps lines within limit bytes; a longer output becomes one
// truncated line.
func capLines(lines []string, limit int) []string {
	joined := strings.Join(lines, "\n")
	if len(joined) <= limit {
		return lines
	}
	return []string{notification.Truncate(joined, limit)}
}

func (p *Pagerduty) buildPayload(m notification.Message) pagerdutyPayload {
	payload := pagerdutyPayload{
		RoutingKey:  p.integrationKey,
		EventAction: "trigger",
		DedupKey:    m.AlertKeyMax(p.clusterName, maxDedupKey),
	}
	if m.Resolved() {
		payload.EventAction = "resolve"
		return payload
	}
	source := p.clusterName
	if source == "" {
		source = "kwatch"
	}
	payload.Payload = &pagerdutyPayloadDetail{
		Summary:  notification.Truncate(m.ShortText(), summaryLimit),
		Source:   source,
		Severity: pagerdutySeverity(m),
		CustomDetail: pagerdutyCustomDetails{
			Cluster: p.clusterName,
			Details: notification.Truncate(
				strings.TrimSpace(m.NoteText()), detailsLimit),
			Output:     capLines(m.Output, detailsLimit),
			Namespaces: m.Route.Namespaces,
			Reasons:    m.Route.Reasons,
		},
	}
	return payload
}

// maxDedupKey is the longest dedup_key the Events API accepts.
const maxDedupKey = 255
