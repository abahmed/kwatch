package ilert

import (
	"encoding/json"
	"fmt"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
)

const ilertAPIURL = "https://api.ilert.com/api/v1/events/push/%s"

type ilertPayload struct {
	EventType string `json:"eventType"`
	Summary   string `json:"summary"`
	Message   string `json:"message"`
	Priority  string `json:"priority,omitempty"`
}

type Ilert struct {
	url            string
	integrationKey string
	priority       string

	appCfg *config.App
}

// NewIlert returns a new Ilert object
func NewIlert(config map[string]interface{}, appCfg *config.App) *Ilert {
	integrationKey, ok := config["integrationKey"].(string)
	if !ok || len(integrationKey) == 0 {
		klog.InfoS("initializing ilert with empty integrationKey")
		return nil
	}

	priority := "HIGH"
	if p, ok := config["priority"].(string); ok && len(p) > 0 {
		priority = p
	}

	klog.InfoS("initializing ilert", "priority", priority)

	return &Ilert{
		url:            fmt.Sprintf(ilertAPIURL, integrationKey),
		integrationKey: integrationKey,
		priority:       priority,
		appCfg:         appCfg,
	}
}

// Name returns name of the provider
func (i *Ilert) Name() string {
	return "Ilert"
}

// SendEvent sends event to the provider
func (i *Ilert) SendEvent(e *event.Event) error {
	return i.SendMessage(e.FormatText(i.appCfg.ClusterName, ""))
}

// SendMessage sends text message to the provider
func (i *Ilert) SendMessage(msg string) error {
	payload := ilertPayload{
		EventType: "ALERT",
		Summary:   msg,
		Message:   msg,
		Priority:  i.priority,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = util.Post(i.Name(), i.url, body, "application/json", nil)
	return err
}
