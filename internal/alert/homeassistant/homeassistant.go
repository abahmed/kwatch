package homeassistant

import (
	"encoding/json"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
)

const defaultHomeAssistantURL = "http://localhost:8123"
const defaultNotifyService = "notify"

type homeAssistantPayload struct {
	Title   string `json:"title,omitempty"`
	Message string `json:"message"`
}

type HomeAssistant struct {
	url     string
	token   string
	service string

	appCfg *config.App
}

// NewHomeAssistant returns a new HomeAssistant object
func NewHomeAssistant(config map[string]interface{}, appCfg *config.App) *HomeAssistant {
	token, ok := config["token"].(string)
	if !ok || len(token) == 0 {
		klog.InfoS("initializing homeassistant with empty token")
		return nil
	}

	server := defaultHomeAssistantURL
	if s, ok := config["url"].(string); ok && len(s) > 0 {
		server = s
	}

	service := defaultNotifyService
	if s, ok := config["service"].(string); ok && len(s) > 0 {
		service = s
	}

	klog.InfoS("initializing homeassistant", "url", server, "service", service)

	return &HomeAssistant{
		url:     strings.TrimRight(server, "/") + "/api/services/notify/" + service,
		token:   token,
		service: service,
		appCfg:  appCfg,
	}
}

// Name returns name of the provider
func (h *HomeAssistant) Name() string {
	return "HomeAssistant"
}

// SendEvent sends event to the provider
func (h *HomeAssistant) SendEvent(e *event.Event) error {
	msg := e.FormatText(h.appCfg.ClusterName, "")
	return h.SendMessage(msg)
}

// SendMessage sends text message to the provider
func (h *HomeAssistant) SendMessage(msg string) error {
	payload := homeAssistantPayload{
		Title:   "kwatch alert",
		Message: msg,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = util.Post(h.Name(), h.url, body, "application/json", map[string]string{
		"Authorization": "Bearer " + h.token,
	})
	return err
}
