package zapier

import (
	"encoding/json"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
)

type zapierPayload struct {
	Title   string `json:"title,omitempty"`
	Message string `json:"message"`
	Text    string `json:"text,omitempty"`
}

type Zapier struct {
	url   string
	title string

	appCfg *config.App
}

// NewZapier returns a new Zapier object
func NewZapier(config map[string]interface{}, appCfg *config.App) *Zapier {
	url, ok := config["url"].(string)
	if !ok || len(url) == 0 {
		klog.InfoS("initializing zapier with empty url")
		return nil
	}

	title, _ := config["title"].(string)

	klog.InfoS("initializing zapier", "title", title)

	return &Zapier{
		url:    url,
		title:  title,
		appCfg: appCfg,
	}
}

// Name returns name of the provider
func (z *Zapier) Name() string {
	return "Zapier"
}

// SendEvent sends event to the provider
func (z *Zapier) SendEvent(e *event.Event) error {
	msg := e.FormatText(z.appCfg.ClusterName, "")
	return z.SendMessage(msg)
}

// SendMessage sends text message to the provider
func (z *Zapier) SendMessage(msg string) error {
	payload := zapierPayload{
		Title:   z.title,
		Message: msg,
		Text:    msg,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = util.Post(z.Name(), z.url, body, "application/json", nil)
	return err
}
