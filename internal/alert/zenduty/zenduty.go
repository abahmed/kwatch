package zenduty

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/k8s"
	"k8s.io/klog/v2"
)

const (
	defaultZendutyTitle = "kwatch detected a crash in pod: %s"
	defaultZendutyText  = "There is an issue with container (%s) in pod (%s)"
	zendutyAPIURL       = "https://www.zenduty.com/api/events"
)

var AlertTypes = []string{
	"critical",
	"acknowledged",
	"resolved",
	"error",
	"warning",
	"info",
}

type Zenduty struct {
	integrationkey string
	url            string
	alertType      string

	// reference for general app configuration
	appCfg *config.App
}

type zendutyPayload struct {
	Message   string `json:"message"`
	Summary   string `json:"summary"`
	AlertType string `json:"alert_type"`
	EntityID  string `json:"entity_id,omitempty"`
}

// NewZenduty returns new zenduty instance
func NewZenduty(config map[string]interface{}, appCfg *config.App) *Zenduty {
	integrationKey, ok := config["integrationKey"].(string)
	if !ok || len(integrationKey) == 0 {
		klog.InfoS("initializing zenduty with empty webhook url")
		return nil
	}

	klog.InfoS("initializing zenduty with secret apikey")

	// If alert type is not provided, or provided with invalid value
	// it will fallback to critical type
	alertType, ok := config["alertType"].(string)
	if !ok || !slices.Contains(AlertTypes, alertType) {
		alertType = "critical"
	}

	return &Zenduty{
		integrationkey: integrationKey,
		url:            zendutyAPIURL,
		alertType:      alertType,
		appCfg:         appCfg,
	}
}

// Name returns name of the provider
func (m *Zenduty) Name() string {
	return "Zenduty"
}

func (m *Zenduty) UsesEventDelivery() {}

// SendMessage sends text message to the provider
func (m *Zenduty) SendMessage(msg string) error {
	return nil
}

// SendEvent sends event to the provider
func (m *Zenduty) SendEvent(e *event.Event) error {
	if e.Action == "resolved" {
		return m.resolveAlert(e.DedupKey)
	}
	b, err := m.buildMessage(e)
	if err != nil {
		return err
	}
	return m.sendAPI(b)
}

func (m *Zenduty) resolveAlert(entityID string) error {
	payload := zendutyPayload{
		AlertType: "resolved",
		EntityID:  entityID,
		Message:   "resolved",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal zenduty resolve payload: %w", err)
	}
	return m.sendAPI(body)
}

// sendAPI sends http request to Zenduty API
func (m *Zenduty) sendAPI(content []byte) error {
	client := k8s.GetDefaultClient()
	buffer := bytes.NewBuffer(content)
	url := m.url + "/" + m.integrationkey + "/"
	request, err := http.NewRequest(http.MethodPost, url, buffer)
	if err != nil {
		return err
	}

	request.Header.Set("Content-Type", "application/json")

	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode != 201 {
		if response.StatusCode == http.StatusTooManyRequests {
			return event.CheckHTTPResponse(response, "zenduty")
		}
		body, _ := io.ReadAll(response.Body)
		return fmt.Errorf(
			"call to zenduty alert returned status code %d: %s",
			response.StatusCode,
			string(body))
	}

	return nil
}

func (m *Zenduty) buildMessage(e *event.Event) ([]byte, error) {
	payload := zendutyPayload{
		AlertType: m.alertType,
		EntityID:  e.DedupKey,
	}

	logs := constant.DefaultLogs
	if len(e.Logs) > 0 {
		logs = (e.Logs)
	}

	events := constant.DefaultEvents
	if len(e.Events) > 0 {
		events = (e.Events)
	}

	payload.Message = fmt.Sprintf(defaultZendutyTitle, e.PodName)
	payload.Summary = fmt.Sprintf(
		"An alert has been triggered for\n\n"+
			"cluster: %s\n"+
			"Node Name: %s\n"+
			"Pod Name: %s\n"+
			"Container: %s\n"+
			"Namespace: %s\n"+
			"Reason: %s\n\n"+
			"Events:\n%s\n\n"+
			"Logs:\n%s\n\n",
		m.appCfg.ClusterName,
		e.NodeName,
		e.PodName,
		e.ContainerName,
		e.Namespace,
		e.Reason,
		events,
		logs,
	)

	str, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal zenduty payload: %w", err)
	}
	return str, nil
}
