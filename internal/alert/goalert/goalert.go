package goalert

import (
	"encoding/json"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
)

const (
	goalertAPIURL  = "https://goalert.example.com"
	goalertAPIPath = "/api/v2/events"
)

type goalertPayload struct {
	Type    string `json:"type"`
	Service string `json:"serviceID,omitempty"`
	Message string `json:"message,omitempty"`
}

type Goalert struct {
	url       string
	token     string
	serviceID string

	appCfg *config.App
}

// NewGoalert returns a new Goalert object
func NewGoalert(config map[string]interface{}, appCfg *config.App) *Goalert {
	token, ok := config["token"].(string)
	if !ok || len(token) == 0 {
		klog.InfoS("initializing goalert with empty token")
		return nil
	}

	serviceID, ok := config["serviceId"].(string)
	if !ok || len(serviceID) == 0 {
		klog.InfoS("initializing goalert with empty serviceId")
		return nil
	}

	server := goalertAPIURL
	if u, ok := config["url"].(string); ok && len(u) > 0 {
		server = u
	}

	klog.InfoS("initializing goalert", "url", server, "serviceID", serviceID)

	return &Goalert{
		url:       strings.TrimRight(server, "/") + goalertAPIPath,
		token:     token,
		serviceID: serviceID,
		appCfg:    appCfg,
	}
}

// Name returns name of the provider
func (s *Goalert) Name() string {
	return "GoAlert"
}

// SendEvent sends event to the provider
func (s *Goalert) SendEvent(e *event.Event) error {
	msg := e.FormatText(s.appCfg.ClusterName, "")
	return s.SendMessage(msg)
}

// SendMessage sends text message to the provider
func (s *Goalert) SendMessage(msg string) error {
	payload := goalertPayload{
		Type:    "incident.create",
		Service: s.serviceID,
		Message: msg,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = util.Post(s.Name(), s.url, body, "application/json", map[string]string{
		"Authorization": "Bearer " + s.token,
	})
	return err
}
