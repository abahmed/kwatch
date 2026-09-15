package homeassistant

import (
	"context"
	"encoding/json"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
)

const defaultHomeAssistantURL = "http://localhost:8123"
const defaultNotifyService = "notify"

type homeAssistantPayload struct {
	Title   string `json:"title,omitempty"`
	Message string `json:"message"`
}

type HomeAssistant struct {
	sender  transport.Sender
	url     string
	token   string
	service string

	clusterName string
}

// NewHomeAssistant returns a new HomeAssistant object

func NewHomeAssistant(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *HomeAssistant {
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
		sender: transport.NewSender(dependencies),
		url: strings.TrimRight(server, "/") +
			"/api/services/notify/" + service,
		token:       token,
		service:     service,
		clusterName: clusterName,
	}
}

// Name returns name of the provider
func (h *HomeAssistant) Name() string {
	return "HomeAssistant"
}

// SendEvent sends event to the provider
func (h *HomeAssistant) SendEvent(ctx context.Context, e *event.Event) error {
	msg := e.FormatText(h.clusterName, "")
	return h.SendMessage(ctx, msg)
}

// SendMessage sends text message to the provider
func (h *HomeAssistant) SendMessage(ctx context.Context, msg string) error {
	payload := homeAssistantPayload{
		Title:   "kwatch alert",
		Message: msg,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = h.sender.Send(ctx, transport.Request{
		Provider: h.Name(), URL: h.url, Body: body,
		ContentType: "application/json", Headers: map[string]string{
			"Authorization": "Bearer " + h.token,
		},
	})
	return err
}
