package datadog

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

const defaultDatadogSite = "datadoghq.com"

type datadogPayload struct {
	Title     string   `json:"title"`
	Text      string   `json:"text"`
	Tags      []string `json:"tags,omitempty"`
	AlertType string   `json:"alert_type,omitempty"`

	AggregationKey string `json:"aggregation_key,omitempty"`
}

type Datadog struct {
	sender    transport.Sender
	url       string
	apiKey    string
	appKey    string
	title     string
	alertType string
	tags      []string

	clusterName string
}

// NewDatadog returns a new Datadog object

func NewDatadog(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Datadog {
	apiKey, ok := config["apiKey"].(string)
	if !ok || len(apiKey) == 0 {
		klog.InfoS("initializing datadog with empty apiKey")
		return nil
	}

	site := defaultDatadogSite
	if s, ok := config["site"].(string); ok && len(s) > 0 {
		site = s
	}

	appKey, _ := config["applicationKey"].(string)
	title, _ := config["title"].(string)
	// Datadog rejects a title over its limit, so a long configured title
	// is cut once here instead of failing every delivery.
	title = notification.Truncate(title, maxTitleBytes)

	// An unset alertType lets each incident carry its own severity. An
	// unknown one is ignored for the same reason: Datadog would reject it.
	alertType, _ := config["alertType"].(string)
	if alertType != "" && !validAlertTypes[alertType] {
		klog.InfoS("ignoring invalid datadog alertType",
			"alertType", alertType)
		alertType = ""
	}

	var tags []string
	if raw, ok := config["tags"].([]interface{}); ok {
		for _, tag := range raw {
			if s, ok := tag.(string); ok && len(s) > 0 {
				tags = append(tags, s)
			}
		}
	}

	klog.InfoS("initializing datadog", "site", site, "title", title)

	return &Datadog{
		sender:      transport.NewSender(dependencies),
		url:         fmt.Sprintf("https://api.%s/api/v1/events", site),
		apiKey:      apiKey,
		appKey:      appKey,
		title:       title,
		alertType:   alertType,
		tags:        tags,
		clusterName: clusterName,
	}
}

// Name returns name of the provider
func (d *Datadog) Name() string {
	return "Datadog"
}

// Datadog event field limits.
const (
	maxTitleBytes = 100
	maxTextBytes  = 4000
)

// validAlertTypes are the alert_type values the Datadog events API accepts.
var validAlertTypes = map[string]bool{
	"error": true, "warning": true, "info": true, "success": true,
	"user_update": true, "recommendation": true, "snapshot": true,
}

// SendIncident posts a Datadog event aggregated per kwatch incident;
// resolves are posted with alert_type success.
func (d *Datadog) SendIncident(
	ctx context.Context, m notification.Message,
) error {
	body, err := json.Marshal(d.buildPayload(m))
	if err != nil {
		return err
	}
	headers := map[string]string{
		"DD-API-KEY": d.apiKey,
	}
	if len(d.appKey) > 0 {
		headers["DD-APPLICATION-KEY"] = d.appKey
	}
	_, err = d.sender.Send(ctx, transport.Request{
		Provider: d.Name(), URL: d.url, Body: body,
		ContentType: "application/json", Headers: headers,
	})
	return err
}

func (d *Datadog) buildPayload(m notification.Message) datadogPayload {
	title := d.title
	if len(title) == 0 {
		title = notification.Truncate(m.ShortText(), maxTitleBytes)
	}
	text := m.NoteText()
	if len(m.Output) > 0 {
		text += "\n\nLast output:\n" + strings.Join(m.Output, "\n")
	}
	return datadogPayload{
		Title:          title,
		Text:           notification.Truncate(text, maxTextBytes),
		Tags:           d.tags,
		AlertType:      d.alertTypeFor(m),
		AggregationKey: m.AlertKey(d.clusterName),
	}
}

// alertTypeFor is "success" for a resolve, "info" for a notice, the
// configured alert type when set, and the incident's severity otherwise.
func (d *Datadog) alertTypeFor(m notification.Message) string {
	switch {
	case m.Resolved():
		return "success"
	case m.IsNotice():
		return "info"
	case d.alertType != "":
		return d.alertType
	}
	switch m.Route.Severity {
	case "warning":
		return "warning"
	case "info":
		return "info"
	}
	return "error"
}

// SendMessage sends a plain notice as an informational event.
func (d *Datadog) SendMessage(ctx context.Context, msg string) error {
	return d.SendIncident(ctx, notification.Notice(msg))
}
