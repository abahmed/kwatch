package datadog

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
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

	alertType := "error"
	if t, ok := config["alertType"].(string); ok && len(t) > 0 {
		alertType = t
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

// SendEvent sends event to the provider
// UsesEventDelivery routes incidents through SendEvent, which carries the
// action and a stable key so Datadog groups one incident's events.
func (d *Datadog) UsesEventDelivery() {}

// SendEvent posts a Datadog event aggregated per kwatch incident; resolves
// are posted with alert_type success.
func (d *Datadog) SendEvent(ctx context.Context, e *event.Event) error {
	title := d.title
	if len(title) == 0 {
		title = e.AlertTitle(100)
	}
	alertType := d.alertType
	switch {
	case e.IsResolve():
		alertType = "success"
	case e.IsNotice():
		alertType = "info"
	}
	payload := datadogPayload{
		Title:          title,
		Text:           e.AlertBody(d.clusterName),
		Tags:           d.tags,
		AlertType:      alertType,
		AggregationKey: e.AlertKey(),
	}
	body, err := json.Marshal(payload)
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

// SendMessage sends a plain notice as an informational event.
func (d *Datadog) SendMessage(ctx context.Context, msg string) error {
	return d.SendEvent(ctx, &event.Event{PodName: msg, Reason: "notify"})
}
