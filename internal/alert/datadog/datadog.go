package datadog

import (
	"encoding/json"
	"fmt"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
)

const defaultDatadogSite = "datadoghq.com"

type datadogPayload struct {
	Title     string   `json:"title"`
	Text      string   `json:"text"`
	Tags      []string `json:"tags,omitempty"`
	AlertType string   `json:"alert_type,omitempty"`
}

type Datadog struct {
	url       string
	apiKey    string
	appKey    string
	title     string
	alertType string
	tags      []string

	appCfg *config.App
}

// NewDatadog returns a new Datadog object
func NewDatadog(config map[string]interface{}, appCfg *config.App) *Datadog {
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
		url:       fmt.Sprintf("https://api.%s/api/v1/events", site),
		apiKey:    apiKey,
		appKey:    appKey,
		title:     title,
		alertType: alertType,
		tags:      tags,
		appCfg:    appCfg,
	}
}

// Name returns name of the provider
func (d *Datadog) Name() string {
	return "Datadog"
}

// SendEvent sends event to the provider
func (d *Datadog) SendEvent(e *event.Event) error {
	msg := e.FormatText(d.appCfg.ClusterName, "")
	return d.SendMessage(msg)
}

// SendMessage sends text message to the provider
func (d *Datadog) SendMessage(msg string) error {
	title := d.title
	if len(title) == 0 {
		title = "kwatch alert"
	}

	payload := datadogPayload{
		Title:     title,
		Text:      msg,
		Tags:      d.tags,
		AlertType: d.alertType,
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

	_, err = util.Post(d.Name(), d.url, body, "application/json", headers)
	return err
}
