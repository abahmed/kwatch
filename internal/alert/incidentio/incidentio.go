package incidentio

import (
	"encoding/json"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
)

type incidentioPayload struct {
	EventType string                 `json:"event_type"`
	Source    string                 `json:"source"`
	Severity  string                 `json:"severity,omitempty"`
	Message   string                 `json:"message"`
	Payload   map[string]interface{} `json:"payload,omitempty"`
}

type Incidentio struct {
	url    string
	apiKey string

	appCfg *config.App
}

// NewIncidentio returns a new Incidentio object
func NewIncidentio(config map[string]interface{}, appCfg *config.App) *Incidentio {
	url, ok := config["url"].(string)
	if !ok || len(url) == 0 {
		klog.InfoS("initializing incidentio with empty url")
		return nil
	}

	apiKey, _ := config["apiKey"].(string)

	klog.InfoS("initializing incidentio")

	return &Incidentio{
		url:    url,
		apiKey: apiKey,
		appCfg: appCfg,
	}
}

// Name returns name of the provider
func (i *Incidentio) Name() string {
	return "Incident.io"
}

// SendEvent sends event to the provider
func (i *Incidentio) SendEvent(e *event.Event) error {
	payload := incidentioPayload{
		EventType: "kwatch.incident",
		Source:    "kwatch",
		Severity:  string(e.Severity),
		Message:   e.FormatText(i.appCfg.ClusterName, ""),
		Payload: map[string]interface{}{
			"pod":       e.PodName,
			"container": e.ContainerName,
			"namespace": e.Namespace,
			"node":      e.NodeName,
			"reason":    e.Reason,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	return i.send(body)
}

// SendMessage sends text message to the provider
func (i *Incidentio) SendMessage(msg string) error {
	payload := incidentioPayload{
		EventType: "kwatch.incident",
		Source:    "kwatch",
		Message:   msg,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	return i.send(body)
}

func (i *Incidentio) send(body []byte) error {
	headers := map[string]string{}
	if len(i.apiKey) > 0 {
		headers["Authorization"] = "Bearer " + i.apiKey
	}

	_, err := util.Post(i.Name(), i.url, body, "application/json", headers)
	return err
}
