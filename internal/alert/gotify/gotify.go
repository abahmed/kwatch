package gotify

import (
	"encoding/json"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
)

const gotifyAPIURL = "/message"

type gotifyPayload struct {
	Title    string `json:"title,omitempty"`
	Message  string `json:"message"`
	Priority int    `json:"priority,omitempty"`
}

type Gotify struct {
	url      string
	token    string
	title    string
	priority int

	appCfg *config.App
}

// NewGotify returns a new Gotify object
func NewGotify(config map[string]interface{}, appCfg *config.App) *Gotify {
	server, ok := config["url"].(string)
	if !ok || len(server) == 0 {
		klog.InfoS("initializing gotify with empty url")
		return nil
	}

	token, ok := config["token"].(string)
	if !ok || len(token) == 0 {
		klog.InfoS("initializing gotify with empty token")
		return nil
	}

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

	klog.InfoS("initializing gotify", "url", server, "title", title)

	return &Gotify{
		url:      strings.TrimRight(server, "/") + gotifyAPIURL,
		token:    token,
		title:    title,
		priority: priority,
		appCfg:   appCfg,
	}
}

// Name returns name of the provider
func (g *Gotify) Name() string {
	return "Gotify"
}

// SendEvent sends event to the provider
func (g *Gotify) SendEvent(e *event.Event) error {
	msg := e.FormatText(g.appCfg.ClusterName, "")
	return g.SendMessage(msg)
}

// SendMessage sends text message to the provider
func (g *Gotify) SendMessage(msg string) error {
	payload := gotifyPayload{
		Title:    g.title,
		Message:  msg,
		Priority: g.priority,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = util.Post(g.Name(), g.url, body, "application/json", map[string]string{
		"X-Gotify-Key": g.token,
	})
	return err
}
