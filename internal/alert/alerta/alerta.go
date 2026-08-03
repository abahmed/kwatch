package alerta

import (
	"encoding/json"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
)

const alertaAPIPath = "/api/alert"

type alertaPayload struct {
	Resource    string   `json:"resource"`
	Event       string   `json:"event"`
	Environment string   `json:"environment"`
	Severity    string   `json:"severity"`
	Service     []string `json:"service"`
	Text        string   `json:"text,omitempty"`
}

type Alerta struct {
	url         string
	apiKey      string
	environment string
	service     string

	appCfg *config.App
}

// NewAlerta returns a new Alerta object
func NewAlerta(config map[string]interface{}, appCfg *config.App) *Alerta {
	url, ok := config["url"].(string)
	if !ok || len(url) == 0 {
		klog.InfoS("initializing alerta with empty url")
		return nil
	}

	apiKey, ok := config["apiKey"].(string)
	if !ok || len(apiKey) == 0 {
		klog.InfoS("initializing alerta with empty apiKey")
		return nil
	}

	environment, _ := config["environment"].(string)
	if len(environment) == 0 {
		environment = "Production"
	}

	service, _ := config["service"].(string)
	if len(service) == 0 {
		service = "kwatch"
	}

	klog.InfoS("initializing alerta", "url", url, "environment", environment)

	return &Alerta{
		url:         strings.TrimRight(url, "/") + alertaAPIPath,
		apiKey:      apiKey,
		environment: environment,
		service:     service,
		appCfg:      appCfg,
	}
}

// Name returns name of the provider
func (s *Alerta) Name() string {
	return "Alerta"
}

// SendEvent sends event to the provider
func (s *Alerta) SendEvent(e *event.Event) error {
	msg := e.FormatText(s.appCfg.ClusterName, "")
	return s.SendMessage(msg)
}

// SendMessage sends text message to the provider
func (s *Alerta) SendMessage(msg string) error {
	resource := "kwatch"
	if len(s.appCfg.ClusterName) > 0 {
		resource = "kwatch/" + s.appCfg.ClusterName
	}

	payload := alertaPayload{
		Resource:    resource,
		Event:       "kwatch",
		Environment: s.environment,
		Severity:    "critical",
		Service:     []string{s.service},
		Text:        msg,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = util.Post(s.Name(), s.url, body, "application/json", map[string]string{
		"Authorization": "Key " + s.apiKey,
	})
	return err
}
