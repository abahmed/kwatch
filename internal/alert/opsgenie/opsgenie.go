package opsgenie

import (
	"encoding/json"
	"fmt"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
)

const (
	defaultOpsgenieTitle = "kwatch detected a crash in pod: %s"
	defaultOpsgenieText  = "There is an issue with container (%s) in pod (%s)"
	opsgenieAPIURL       = "https://api.opsgenie.com/v2/alerts"
	opsgenieCloseURL     = "https://api.opsgenie.com/v2/alerts/%s/close" +
		"?identifierType=alias"
)

type Opsgenie struct {
	apikey   string
	url      string
	closeURL string
	title    string
	text     string

	// reference for general app configuration
	appCfg *config.App
}

type ogPayload struct {
	Message     string      `json:"message"`
	Description string      `json:"description"`
	Details     interface{} `json:"details"`
	Priority    string      `json:"priority"`
	Alias       string      `json:"alias,omitempty"`
}

// NewOpsgenie returns new opsgenie instance
func NewOpsgenie(config map[string]interface{}, appCfg *config.App) *Opsgenie {
	apiKey, ok := config["apiKey"].(string)
	if !ok || len(apiKey) == 0 {
		klog.InfoS("initializing opsgenie with empty webhook url")
		return nil
	}

	klog.InfoS("initializing opsgenie with secret apikey")

	title, _ := config["title"].(string)
	text, _ := config["text"].(string)

	return &Opsgenie{
		apikey:   apiKey,
		url:      opsgenieAPIURL,
		closeURL: opsgenieCloseURL,
		title:    title,
		text:     text,
		appCfg:   appCfg,
	}
}

// Name returns name of the provider
func (o *Opsgenie) Name() string {
	return "Opsgenie"
}

func (o *Opsgenie) UsesEventDelivery() {}

// SendMessage sends text message to the provider
func (o *Opsgenie) SendMessage(msg string) error {
	return nil
}

// SendEvent sends event to the provider
func (o *Opsgenie) SendEvent(e *event.Event) error {
	if e.Action == "resolved" && e.DedupKey != "" {
		return o.closeAlert(e.DedupKey)
	}
	b, err := o.buildMessage(e)
	if err != nil {
		return err
	}
	return o.sendAPI(b)
}

func (o *Opsgenie) closeAlert(alias string) error {
	_, err := util.Send(util.Request{
		Provider: "Opsgenie",
		URL:      fmt.Sprintf(o.closeURL, alias),
		Body:     []byte(`{}`),
		Headers:  map[string]string{"Authorization": "GenieKey " + o.apikey},
	})
	return err
}

// sendAPI sends http request to Opsgenie API
func (o *Opsgenie) sendAPI(content []byte) error {
	_, err := util.Send(util.Request{
		Provider: "Opsgenie",
		URL:      o.url,
		Body:     content,
		Headers:  map[string]string{"Authorization": "GenieKey " + o.apikey},
	})
	return err
}

func (o *Opsgenie) buildMessage(e *event.Event) ([]byte, error) {
	payload := ogPayload{
		Priority: "P1",
	}

	logs := strings.TrimSpace(e.Logs)
	events := strings.TrimSpace(e.Events)

	// use custom title if it's provided, otherwise use default
	title := o.title
	if len(title) == 0 {
		title = fmt.Sprintf(defaultOpsgenieTitle, e.PodName)
	}
	payload.Message = title

	// use custom text if it's provided, otherwise use default
	text := o.text
	if len(text) == 0 {
		text = fmt.Sprintf(defaultOpsgenieText, e.ContainerName, e.PodName)
	}

	payload.Description = text
	payload.Alias = e.DedupKey
	details := map[string]string{}
	if o.appCfg.ClusterName != "" {
		details["Cluster"] = o.appCfg.ClusterName
	}
	if e.PodName != "" {
		details["Name"] = e.PodName
	}
	if e.ContainerName != "" {
		details["Container"] = e.ContainerName
	}
	if e.Namespace != "" {
		details["Namespace"] = e.Namespace
	}
	if e.NodeName != "" {
		details["Node"] = e.NodeName
	}
	if e.Reason != "" {
		details["Reason"] = e.Reason
	}
	if len(events) > 0 {
		details["Events"] = events
	}
	if len(logs) > 0 {
		details["Logs"] = logs
	}
	payload.Details = details

	str, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal opsgenie payload: %w", err)
	}
	return str, nil
}
