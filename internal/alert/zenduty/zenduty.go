package zenduty

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/model"
)

const (
	defaultZendutyTitle = "kwatch detected a crash in pod: %s"
	defaultZendutyText  = "There is an issue with container (%s) in pod (%s)"
	zendutyAPIURL       = "https://www.zenduty.com/api/events"
)

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

func (z *Zenduty) UsesEventDelivery() {}

// SendMessage sends text message to the provider
func (z *Zenduty) SendMessage(ctx context.Context, msg string) error {
	return nil
}

// SendEvent sends event to the provider
func (z *Zenduty) SendEvent(ctx context.Context, e *event.Event) error {
	if e.Action == "resolved" {
		return z.resolveAlert(ctx, e.DedupKey)
	}
	b, err := z.buildMessage(e)
	if err != nil {
		return err
	}
	return z.sendAPI(ctx, b)
}

func (z *Zenduty) resolveAlert(
	ctx context.Context,
	entityID string,
) error {
	payload := zendutyPayload{
		AlertType: "resolved",
		EntityID:  entityID,
		Message:   "resolved",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal zenduty resolve payload: %w", err)
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

// alertTypeFor is the operator's configured alert type when they set one,
// and the incident's own severity otherwise.
func (z *Zenduty) alertTypeFor(sev model.Severity) string {
	if z.alertType != "" {
		return z.alertType
	}
	switch sev {
	case model.SeverityCritical:
		return "critical"
	case model.SeverityHigh:
		return "error"
	case model.SeverityMedium, model.SeverityWarning:
		return "warning"
	case model.SeverityNormal:
		return "info"
	}
	return "error"
}

func (z *Zenduty) buildMessage(e *event.Event) ([]byte, error) {
	payload := zendutyPayload{
		AlertType: z.alertTypeFor(e.Severity),
		EntityID:  e.DedupKey,
	}

	msg := defaultZendutyTitle
	if narrative := strings.TrimSpace(e.Narrative); narrative != "" {
		msg = zendutyFirstLine(narrative, 130)
	} else if e.PodName != "" {
		msg = fmt.Sprintf(defaultZendutyTitle, e.PodName)
	}
	payload.Message = msg

	var summaryParts []string
	summaryParts = append(
		summaryParts,
		fmt.Sprintf("Reason: %s", format.OrDefault(e.Reason, "unknown")),
	)
	if e.PodName != "" {
		summaryParts = append(summaryParts, fmt.Sprintf("Pod: %s", e.PodName))
	}
	if e.ContainerName != "" {
		summaryParts = append(
			summaryParts,
			fmt.Sprintf("Container: %s", e.ContainerName),
		)
	}
	if e.Namespace != "" {
		summaryParts = append(
			summaryParts,
			fmt.Sprintf("Namespace: %s", e.Namespace),
		)
	}
	if e.NodeName != "" {
		summaryParts = append(summaryParts, fmt.Sprintf("Node: %s", e.NodeName))
	}
	if z.clusterName != "" {
		summaryParts = append(
			summaryParts,
			fmt.Sprintf("Cluster: %s", z.clusterName),
		)
	}

	summary := strings.Join(summaryParts, " · ")
	if narrative := strings.TrimSpace(e.Narrative); narrative != "" {
		summary = narrative
	}

	if e.Narrative == "" && e.IncludeLogs {
		logs := strings.TrimSpace(e.Logs)
		if len(logs) > 0 {
			summary += "\n\nLogs:\n" + logs
		}
	}

	if e.Narrative == "" && e.IncludeEvents {
		events := strings.TrimSpace(e.Events)
		if len(events) > 0 {
			summary += "\n\nEvents:\n" + events
		}
	}

	payload.Summary = summary

	str, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal zenduty payload: %w", err)
	}
	return str, nil
}

func zendutyFirstLine(value string, limit int) string {
	line := strings.SplitN(strings.TrimSpace(value), "\n", 2)[0]
	if len(line) <= limit {
		return line
	}
	return line[:limit-1] + "…"
}
