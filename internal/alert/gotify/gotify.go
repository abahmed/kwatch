package gotify

import (
	"context"
	"encoding/json"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

const gotifyAPIURL = "/message"

type gotifyPayload struct {
	Title    string `json:"title,omitempty"`
	Message  string `json:"message"`
	Priority int    `json:"priority,omitempty"`
}

type Gotify struct {
	sender   transport.Sender
	url      string
	token    string
	title    string
	priority int

	clusterName string
}

// NewGotify returns a new Gotify object

func NewGotify(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Gotify {
	server, ok := config["url"].(string)
	if !ok || len(server) == 0 {
		klog.InfoS("initializing gotify with empty url")
		return nil
	}

	if !transport.ValidEndpoint(server) {
		klog.InfoS("initializing gotify with an invalid url",
			"setting", "url")
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

	klog.InfoS("initializing gotify",
		"url", transport.LogURL(server),
		"title", title)

	return &Gotify{
		sender:      transport.NewSender(dependencies),
		url:         strings.TrimRight(server, "/") + gotifyAPIURL,
		token:       token,
		title:       title,
		priority:    priority,
		clusterName: clusterName,
	}
}

// Name returns name of the provider
func (g *Gotify) Name() string {
	return "Gotify"
}

// SendIncident sends the incident's one-line lead as the push message.
// Open incidents use the configured priority; a resolve uses the server
// default priority.
func (g *Gotify) SendIncident(
	ctx context.Context, m notification.Message,
) error {
	priority := g.priority
	if m.Resolved() {
		priority = 0
	}
	return g.send(ctx, m.ShortText(), priority)
}

// SendMessage sends text message to the provider
func (g *Gotify) SendMessage(ctx context.Context, msg string) error {
	return g.send(ctx, msg, g.priority)
}

func (g *Gotify) send(ctx context.Context, msg string, priority int) error {
	payload := gotifyPayload{
		Title:    g.title,
		Message:  msg,
		Priority: priority,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = g.sender.Send(ctx, transport.Request{
		Provider: g.Name(), URL: g.url, Body: body,
		ContentType: "application/json", Headers: map[string]string{
			"X-Gotify-Key": g.token,
		},
	})
	return err
}
