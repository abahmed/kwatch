package n8n

import (
	"encoding/json"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
)

type n8nPayload struct {
	Title   string `json:"title,omitempty"`
	Message string `json:"message"`
	Text    string `json:"text,omitempty"`
}

type N8n struct {
	url   string
	token string
	title string

	appCfg *config.App
}

// NewN8n returns a new N8n object
func NewN8n(config map[string]interface{}, appCfg *config.App) *N8n {
	url, ok := config["url"].(string)
	if !ok || len(url) == 0 {
		klog.InfoS("initializing n8n with empty url")
		return nil
	}

	token, _ := config["token"].(string)
	title, _ := config["title"].(string)

	klog.InfoS("initializing n8n", "title", title)

	return &N8n{
		url:    url,
		token:  token,
		title:  title,
		appCfg: appCfg,
	}
}

// Name returns name of the provider
func (n *N8n) Name() string {
	return "N8n"
}

// SendEvent sends event to the provider
func (n *N8n) SendEvent(e *event.Event) error {
	msg := e.FormatText(n.appCfg.ClusterName, "")
	return n.SendMessage(msg)
}

// SendMessage sends text message to the provider
func (n *N8n) SendMessage(msg string) error {
	payload := n8nPayload{
		Title:   n.title,
		Message: msg,
		Text:    msg,
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
