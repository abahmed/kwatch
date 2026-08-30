package wecom

import (
	"encoding/json"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
)

type wecomPayload struct {
	MsgType  string            `json:"msgtype"`
	Markdown map[string]string `json:"markdown"`
}

type Wecom struct {
	webhook string

	appCfg *config.App
}

// NewWecom returns a new Wecom object
func NewWecom(config map[string]interface{}, appCfg *config.App) *Wecom {
	webhook, ok := config["webhook"].(string)
	if !ok || len(webhook) == 0 {
		klog.InfoS("initializing wecom with empty webhook")
		return nil
	}

	klog.InfoS("initializing wecom with webhook configured")

	return &Wecom{
		webhook: webhook,
		appCfg:  appCfg,
	}
}

// Name returns name of the provider
func (s *Wecom) Name() string {
	return "WeCom"
}

// SendEvent sends event to the provider
func (s *Wecom) SendEvent(e *event.Event) error {
	msg := e.FormatText(s.appCfg.ClusterName, "")
	return s.SendMessage(msg)
}

// SendMessage sends text message to the provider
func (s *Wecom) SendMessage(msg string) error {
	payload := wecomPayload{
		MsgType: "markdown",
		Markdown: map[string]string{
			"content": msg,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = util.Post(s.Name(), s.webhook, body, "application/json", nil)
	return err
}
