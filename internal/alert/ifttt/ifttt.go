package ifttt

import (
	"encoding/json"
	"fmt"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
)

const iftttAPIURL = "https://maker.ifttt.com/trigger/%s/with/key/%s"

type iftttPayload struct {
	Value1 string `json:"value1"`
	Value2 string `json:"value2"`
	Value3 string `json:"value3"`
}

type Ifttt struct {
	url   string
	key   string
	event string

	appCfg *config.App
}

// NewIfttt returns a new Ifttt object
func NewIfttt(config map[string]interface{}, appCfg *config.App) *Ifttt {
	key, ok := config["key"].(string)
	if !ok || len(key) == 0 {
		klog.InfoS("initializing ifttt with empty key")
		return nil
	}

	eventName := "kwatch"
	if e, ok := config["event"].(string); ok && len(e) > 0 {
		eventName = e
	}

	klog.InfoS("initializing ifttt", "event", eventName)

	return &Ifttt{
		url:    fmt.Sprintf(iftttAPIURL, eventName, key),
		key:    key,
		event:  eventName,
		appCfg: appCfg,
	}
}

// Name returns name of the provider
func (i *Ifttt) Name() string {
	return "Ifttt"
}

// SendEvent sends event to the provider
func (i *Ifttt) SendEvent(e *event.Event) error {
	msg := e.FormatText(i.appCfg.ClusterName, "")
	return i.SendMessage(msg)
}

// SendMessage sends text message to the provider
func (i *Ifttt) SendMessage(msg string) error {
	payload := iftttPayload{
		Value1: "kwatch",
		Value2: msg,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = util.Post(i.Name(), i.url, body, "application/json", nil)
	return err
}
