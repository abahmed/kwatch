package ntfy

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
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
	sender   transport.Sender
	url      string
	token    string
	title    string
	priority int

	clusterName string
}

// NewNtfy returns a new Ntfy object

func NewNtfy(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Ntfy {
	topic, ok := config["topic"].(string)
	if !ok || len(topic) == 0 {
		klog.InfoS("initializing ntfy with empty topic")
		return nil
	}

	server := defaultNtfyServer
	if s, ok := config["url"].(string); ok && len(s) > 0 {
		if !transport.ValidEndpoint(s) {
			klog.InfoS("initializing ntfy with an invalid url",
				"setting", "url")
			return nil
		}
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

	klog.InfoS("initializing ntfy",
		"url", transport.LogURL(server),
		"title", title)

	return &Ntfy{
		sender: transport.NewSender(dependencies),
		url: strings.TrimRight(server, "/") + "/" +
			url.PathEscape(strings.TrimLeft(topic, "/")),
		token:       token,
		title:       title,
		priority:    priority,
		clusterName: clusterName,
	}
}

// Name returns name of the provider
func (n *Ntfy) Name() string {
	return "Ntfy"
}

// ntfyDefaultPriority is ntfy's normal priority, used for resolves.
const ntfyDefaultPriority = 3

// SendIncident sends the incident's one-line lead as the message. Tags
// are plain words, never ntfy emoji shortcodes, so the status marker at
// the start of the lead stays the only emoji. A resolve is sent at the
// normal priority.
func (n *Ntfy) SendIncident(
	ctx context.Context, m notification.Message,
) error {
	priority := n.priority
	if m.Resolved() {
		priority = ntfyDefaultPriority
	}
	return n.send(ctx, ntfyPayload{
		Title: n.title, Message: m.ShortText(), Priority: priority,
		Tags: []string{"kwatch", "status-" + m.Status.String()},
	})
}

// SendMessage sends text message to the provider
func (n *Ntfy) SendMessage(ctx context.Context, msg string) error {
	return n.send(ctx, ntfyPayload{
		Title:    n.title,
		Message:  msg,
		Priority: n.priority,
		Tags:     []string{"warning"},
	})
}

func (n *Ntfy) send(ctx context.Context, payload ntfyPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	headers := map[string]string{}
	if len(n.token) > 0 {
		headers["Authorization"] = "Bearer " + n.token
	}

	_, err = n.sender.Send(ctx, transport.Request{
		Provider: n.Name(), URL: n.url, Body: body,
		ContentType: "application/json", Headers: headers,
	})
	return err
}
