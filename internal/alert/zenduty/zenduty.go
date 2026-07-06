package zenduty

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/k8s"
	"k8s.io/klog/v2"
)

const (
	defaultZendutyTitle = "kwatch detected a crash in pod: %s"
	defaultZendutyText  = "There is an issue with container (%s) in pod (%s)"
	zendutyAPIURL       = "https://www.zenduty.com/api/events"
)

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

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

	msg := defaultZendutyTitle
	if e.PodName != "" {
		msg = fmt.Sprintf(defaultZendutyTitle, e.PodName)
	}
	payload.Message = msg

	var summaryParts []string
	summaryParts = append(summaryParts, fmt.Sprintf("Reason: %s", orDefault(e.Reason, "unknown")))
	if e.PodName != "" {
		summaryParts = append(summaryParts, fmt.Sprintf("Pod: %s", e.PodName))
	}
	if e.ContainerName != "" {
		summaryParts = append(summaryParts, fmt.Sprintf("Container: %s", e.ContainerName))
	}
	if e.Namespace != "" {
		summaryParts = append(summaryParts, fmt.Sprintf("Namespace: %s", e.Namespace))
	}
	if e.NodeName != "" {
		summaryParts = append(summaryParts, fmt.Sprintf("Node: %s", e.NodeName))
	}
	if m.appCfg.ClusterName != "" {
		summaryParts = append(summaryParts, fmt.Sprintf("Cluster: %s", m.appCfg.ClusterName))
	}

	summary := strings.Join(summaryParts, " · ")

	if e.IncludeLogs {
		logs := strings.TrimSpace(e.Logs)
		if len(logs) > 0 {
			summary += "\n\nLogs:\n" + logs
		}
	}

	if e.IncludeEvents {
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
