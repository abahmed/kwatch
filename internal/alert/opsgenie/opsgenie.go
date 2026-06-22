package opsgenie

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/k8s"
	"k8s.io/klog/v2"
)

const (
	defaultOpsgenieTitle = "kwatch detected a crash in pod: %s"
	defaultOpsgenieText  = "There is an issue with container (%s) in pod (%s)"
	opsgenieAPIURL       = "https://api.opsgenie.com/v2/alerts"
	opsgenieCloseURL     = "https://api.opsgenie.com/v2/alerts/%s/close?identifierType=alias"
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
func (m *Opsgenie) Name() string {
	return "Opsgenie"
}

func (m *Opsgenie) UsesEventDelivery() {}

// SendMessage sends text message to the provider
func (m *Opsgenie) SendMessage(msg string) error {
	return nil
}

// SendEvent sends event to the provider
func (m *Opsgenie) SendEvent(e *event.Event) error {
	if e.Action == "resolved" && e.DedupKey != "" {
		return m.closeAlert(e.DedupKey)
	}
	b, err := m.buildMessage(e)
	if err != nil {
		return err
	}
	return m.sendAPI(b)
}

func (m *Opsgenie) closeAlert(alias string) error {
	client := k8s.GetDefaultClient()
	url := fmt.Sprintf(m.closeURL, alias)
	body := []byte(`{}`)
	buffer := bytes.NewBuffer(body)
	request, err := http.NewRequest(http.MethodPost, url, buffer)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "GenieKey "+m.apikey)

	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode != 202 {
		if response.StatusCode == http.StatusTooManyRequests {
			return event.CheckHTTPResponse(response, "opsgenie")
		}
		body, _ := io.ReadAll(response.Body)
		return fmt.Errorf(
			"call to opsgenie close alert returned status code %d: %s",
			response.StatusCode,
			string(body))
	}
	return nil
}

// sendAPI sends http request to Opsgenie API
func (m *Opsgenie) sendAPI(content []byte) error {
	client := k8s.GetDefaultClient()
	buffer := bytes.NewBuffer(content)
	request, err := http.NewRequest(http.MethodPost, m.url, buffer)
	if err != nil {
		return err
	}

	// set request headers
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "GenieKey "+m.apikey)

	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode != 202 {
		if response.StatusCode == http.StatusTooManyRequests {
			return event.CheckHTTPResponse(response, "opsgenie")
		}
		body, _ := io.ReadAll(response.Body)
		return fmt.Errorf(
			"call to opsgenie alert returned status code %d: %s",
			response.StatusCode,
			string(body))
	}

	return nil
}

func (m *Opsgenie) buildMessage(e *event.Event) ([]byte, error) {
	payload := ogPayload{
		Priority: "P1",
	}

	logs := constant.DefaultLogs
	if len(e.Logs) > 0 {
		logs = (e.Logs)
	}

	events := constant.DefaultEvents
	if len(e.Events) > 0 {
		events = (e.Events)
	}

	// use custom title if it's provided, otherwise use default
	title := m.title
	if len(title) == 0 {
		title = fmt.Sprintf(defaultOpsgenieTitle, e.PodName)
	}
	payload.Message = title

	// use custom text if it's provided, otherwise use default
	text := m.text
	if len(text) == 0 {
		text = fmt.Sprintf(defaultOpsgenieText, e.ContainerName, e.PodName)
	}

	payload.Description = text
	payload.Alias = e.DedupKey
	payload.Details = map[string]string{
		"Cluster":   m.appCfg.ClusterName,
		"Name":      e.PodName,
		"Container": e.ContainerName,
		"Namespace": e.Namespace,
		"Node":      e.NodeName,
		"Reason":    e.Reason,
		"Events":    events,
		"Logs":      logs,
	}

	str, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal opsgenie payload: %w", err)
	}
	return str, nil
}
