package zulip

import (
	"context"
	"encoding/base64"
	"net/url"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
)

const zulipAPIURL = "https://api.zulip.com"

type Zulip struct {
	sender  transport.Sender
	url     string
	email   string
	token   string
	channel string
	title   string

	clusterName string
}

// NewZulip returns a new Zulip object

func NewZulip(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Zulip {
	email, ok := config["email"].(string)
	if !ok || len(email) == 0 {
		klog.InfoS("initializing zulip with empty email")
		return nil
	}

	token, ok := config["token"].(string)
	if !ok || len(token) == 0 {
		klog.InfoS("initializing zulip with empty token")
		return nil
	}

	channel, ok := config["channel"].(string)
	if !ok || len(channel) == 0 {
		klog.InfoS("initializing zulip with empty channel")
		return nil
	}

	server := zulipAPIURL
	if s, ok := config["url"].(string); ok && len(s) > 0 {
		server = s
	}

	title, _ := config["title"].(string)

	klog.InfoS("initializing zulip", "url", server, "channel", channel)

	return &Zulip{
		sender:      transport.NewSender(dependencies),
		url:         strings.TrimRight(server, "/") + "/api/v1/messages",
		email:       email,
		token:       token,
		channel:     channel,
		title:       title,
		clusterName: clusterName,
	}
}

// Name returns name of the provider
func (z *Zulip) Name() string {
	return "Zulip"
}

// SendEvent sends event to the provider
func (z *Zulip) SendEvent(ctx context.Context, e *event.Event) error {
	msg := e.FormatText(z.clusterName, "")
	return z.SendMessage(ctx, msg)
}

// SendMessage sends text message to the provider
func (z *Zulip) SendMessage(ctx context.Context, msg string) error {
	subject := z.title
	if len(subject) == 0 {
		subject = "kwatch alert"
	}

	form := url.Values{}
	form.Set("type", "stream")
	form.Set("to", z.channel)
	form.Set("subject", subject)
	form.Set("content", msg)

	auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(z.email+":"+z.token))

	_, err := z.sender.Send(ctx, transport.Request{
		Provider: z.Name(), URL: z.url, Body: []byte(form.Encode()),
		ContentType: "application/x-www-form-urlencoded", Headers: map[string]string{
			"Authorization": auth,
		},
	})
	return err
}
