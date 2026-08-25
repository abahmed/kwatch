package squadcast

import (
	"encoding/json"
	"fmt"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
)

const squadcastAPIURL = "https://api.squadcast.com/v2/incidents/api/%s"

type squadcastPayload struct {
	Message     string `json:"message"`
	Description string `json:"description"`
	Status      string `json:"status"`
	EventID     string `json:"event_id,omitempty"`
	Severity    string `json:"severity,omitempty"`
}

type Squadcast struct {
	url        string
	serviceKey string

	appCfg *config.App
}

// NewSquadcast returns a new Squadcast object
func NewSquadcast(config map[string]interface{}, appCfg *config.App) *Squadcast {
	serviceKey, ok := config["serviceKey"].(string)
	if !ok || len(serviceKey) == 0 {
		klog.InfoS("initializing squadcast with empty serviceKey")
		return nil
	}

	klog.InfoS("initializing squadcast")

	return &Squadcast{
		url:        fmt.Sprintf(squadcastAPIURL, serviceKey),
		serviceKey: serviceKey,
		appCfg:     appCfg,
	}
}

// Name returns name of the provider
func (s *Squadcast) Name() string {
	return "Squadcast"
}

// SendEvent sends event to the provider
func (s *Squadcast) SendEvent(e *event.Event) error {
	payload := squadcastPayload{
		Message:     e.Reason,
		Description: e.FormatText(s.appCfg.ClusterName, ""),
		Status:      "trigger",
		Severity:    string(e.Severity),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = util.Post(s.Name(), s.url, body, "application/json", nil)
	return err
}

// SendMessage sends text message to the provider
func (s *Squadcast) SendMessage(msg string) error {
	payload := squadcastPayload{
		Message:     "kwatch alert",
		Description: msg,
		Status:      "trigger",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = util.Post(s.Name(), s.url, body, "application/json", nil)
	return err
}
