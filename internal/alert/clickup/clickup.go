package clickup

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
)

const clickupAPIURL = "https://api.clickup.com/api/v2"

type clickupPayload struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Priority    *int   `json:"priority,omitempty"`
}

type Clickup struct {
	sender   transport.Sender
	url      string
	token    string
	listID   string
	priority int

	clusterName string
}

// NewClickup returns a new Clickup object

func NewClickup(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Clickup {
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
		sender:      transport.NewSender(dependencies),
		url:         fmt.Sprintf("%s/list/%s/task", clickupAPIURL, listID),
		token:       token,
		listID:      listID,
		priority:    priority,
		clusterName: clusterName,
	}
}

// Name returns name of the provider
func (c *Clickup) Name() string {
	return "Clickup"
}

// SendEvent sends event to the provider
func (c *Clickup) SendEvent(ctx context.Context, e *event.Event) error {
	msg := e.FormatMarkdown(c.clusterName, "", "\n\n")
	return c.SendMessage(ctx, msg)
}

// SendMessage sends text message to the provider
func (c *Clickup) SendMessage(ctx context.Context, msg string) error {
	title := fmt.Sprintf("kwatch alert: %s", c.clusterName)
	if c.clusterName == "" {
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

	_, err = c.sender.Send(ctx, transport.Request{
		Provider: c.Name(), URL: c.url, Body: body,
		ContentType: "application/json", Headers: map[string]string{
			"Authorization": c.token,
		},
	})
	return err
}
