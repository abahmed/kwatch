package clickup

import (
	"encoding/json"
	"fmt"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
)

const clickupAPIURL = "https://api.clickup.com/api/v2"

type clickupPayload struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Priority    *int   `json:"priority,omitempty"`
}

type Clickup struct {
	url      string
	token    string
	listID   string
	priority int

	appCfg *config.App
}

// NewClickup returns a new Clickup object
func NewClickup(config map[string]interface{}, appCfg *config.App) *Clickup {
	token, ok := config["token"].(string)
	if !ok || len(token) == 0 {
		klog.InfoS("initializing clickup with empty token")
		return nil
	}

	listID, ok := config["listId"].(string)
	if !ok || len(listID) == 0 {
		klog.InfoS("initializing clickup with empty listId")
		return nil
	}

	priority := 0
	switch v := config["priority"].(type) {
	case float64:
		priority = int(v)
	case int:
		priority = v
	case int64:
		priority = int(v)
	}

	klog.InfoS("initializing clickup", "listId", listID, "priority", priority)

	return &Clickup{
		url:      fmt.Sprintf("%s/list/%s/task", clickupAPIURL, listID),
		token:    token,
		listID:   listID,
		priority: priority,
		appCfg:   appCfg,
	}
}

// Name returns name of the provider
func (c *Clickup) Name() string {
	return "Clickup"
}

// SendEvent sends event to the provider
func (c *Clickup) SendEvent(e *event.Event) error {
	msg := e.FormatMarkdown(c.appCfg.ClusterName, "", "\n\n")
	return c.SendMessage(msg)
}

// SendMessage sends text message to the provider
func (c *Clickup) SendMessage(msg string) error {
	title := fmt.Sprintf("kwatch alert: %s", c.appCfg.ClusterName)
	if c.appCfg.ClusterName == "" {
		title = "kwatch alert"
	}

	payload := clickupPayload{
		Name:        title,
		Description: msg,
	}
	if c.priority > 0 {
		payload.Priority = &c.priority
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = util.Post(c.Name(), c.url, body, "application/json", map[string]string{
		"Authorization": c.token,
	})
	return err
}
