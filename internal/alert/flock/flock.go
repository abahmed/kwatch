package flock

import (
	"encoding/json"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
)

type flockPayload struct {
	Text string `json:"text"`
}

type Flock struct {
	webhook string

	appCfg *config.App
}

// NewFlock returns a new Flock object
func NewFlock(config map[string]interface{}, appCfg *config.App) *Flock {
	webhook, ok := config["webhook"].(string)
	if !ok || len(webhook) == 0 {
		klog.InfoS("initializing flock with empty webhook")
		return nil
	}

	klog.InfoS("initializing flock", "webhook", webhook)

	return &Flock{
		webhook: webhook,
		appCfg:  appCfg,
	}
}

// Name returns name of the provider
func (s *Flock) Name() string {
	return "Flock"
}

// SendEvent sends event to the provider
func (s *Flock) SendEvent(e *event.Event) error {
	msg := e.FormatText(s.appCfg.ClusterName, "")
	return s.SendMessage(msg)
}

// SendMessage sends text message to the provider
func (s *Flock) SendMessage(msg string) error {
	payload := flockPayload{
		Text: msg,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = util.Post(s.Name(), s.webhook, body, "application/json", nil)
	return err
}
