package ntfy

import (
	"encoding/json"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
)

const defaultNtfyServer = "https://ntfy.sh"

type ntfyPayload struct {
	Topic    string   `json:"topic,omitempty"`
	Title    string   `json:"title,omitempty"`
	Message  string   `json:"message"`
	Priority int      `json:"priority,omitempty"`
	Tags     []string `json:"tags,omitempty"`
}

type Ntfy struct {
	url      string
	token    string
	title    string
	priority int

	appCfg *config.App
}

// NewNtfy returns a new Ntfy object
func NewNtfy(config map[string]interface{}, appCfg *config.App) *Ntfy {
	topic, ok := config["topic"].(string)
	if !ok || len(topic) == 0 {
		klog.InfoS("initializing ntfy with empty topic")
		return nil
	}

	server := defaultNtfyServer
	if s, ok := config["url"].(string); ok && len(s) > 0 {
		server = s
	}

	token, _ := config["token"].(string)
	title, _ := config["title"].(string)

	priority := 0
	switch v := config["priority"].(type) {
	case float64:
		priority = int(v)
	case int:
		priority = v
	case int64:
		priority = int(v)
	}

	klog.InfoS("initializing ntfy", "url", server, "topic", topic, "title", title)

	return &Ntfy{
		url:      strings.TrimRight(server, "/") + "/" + strings.TrimLeft(topic, "/"),
		token:    token,
		title:    title,
		priority: priority,
		appCfg:   appCfg,
	}
}

// Name returns name of the provider
func (n *Ntfy) Name() string {
	return "Ntfy"
}

// SendEvent sends event to the provider
func (n *Ntfy) SendEvent(e *event.Event) error {
	msg := e.FormatText(n.appCfg.ClusterName, "")
	return n.SendMessage(msg)
}

// SendMessage sends text message to the provider
func (n *Ntfy) SendMessage(msg string) error {
	payload := ntfyPayload{
		Title:    n.title,
		Message:  msg,
		Priority: n.priority,
		Tags:     []string{"warning"},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	headers := map[string]string{}
	if len(n.token) > 0 {
		headers["Authorization"] = "Bearer " + n.token
	}

	_, err = util.Post(n.Name(), n.url, body, "application/json", headers)
	return err
}
