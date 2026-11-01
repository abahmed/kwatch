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
	// resolved is the explicit resolve priority; nil derives it.
	resolved *int

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

	priority, _ := transport.IntSetting(config["priority"])
	resolved := resolvedPriority(config["resolvedPriority"])

	klog.InfoS("initializing gotify",
		"url", transport.LogURL(server),
		"title", title)

	return &Gotify{
		sender:      transport.NewSender(dependencies),
		url:         strings.TrimRight(server, "/") + gotifyAPIURL,
		token:       token,
		title:       title,
		priority:    priority,
		resolved:    resolved,
		clusterName: clusterName,
	}
}

// Name returns name of the provider
func (g *Gotify) Name() string {
	return "Gotify"
}

// SendIncident sends the incident's one-line lead as the push message.
// Open incidents use the configured priority; a resolve uses it capped at
// gotifyNormalPriority, or resolvedPriority when set. With no priority
// configured the server default applies.
func (g *Gotify) SendIncident(
	ctx context.Context, m notification.Message,
) error {
	priority := g.priority
	if m.Resolved() {
		priority = g.resolvePriority()
	}
	return g.send(ctx, m.ShortText(), priority)
}

// gotifyNormalPriority is the most a resolve uses unless
// resolvedPriority says otherwise.
const gotifyNormalPriority = 5

func (g *Gotify) resolvePriority() int {
	if g.priority == 0 && g.resolved == nil {
		return 0
	}
	return transport.ResolvePriority(
		&g.priority, g.resolved, gotifyNormalPriority)
}

// resolvedPriority reads resolvedPriority; a value outside Gotify's
// 0-10 range is ignored with a log line.
func resolvedPriority(value interface{}) *int {
	p := transport.IntSettingPtr(value)
	if p != nil && (*p < 0 || *p > 10) {
		klog.InfoS("ignoring gotify resolvedPriority outside 0-10",
			"resolvedPriority", *p)
		return nil
	}
	return p
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
