package zulip

import (
	"context"
	"encoding/base64"
	"net/url"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/safetext"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

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

	server, ok := config["url"].(string)
	if !ok || len(server) == 0 {
		klog.InfoS("initializing zulip with empty url",
			"setting", "url", "reason", "url is required")
		return nil
	}
	if !validServer(server) {
		klog.InfoS("initializing zulip with an invalid url",
			"setting", "url")
		return nil
	}

	title, _ := config["title"].(string)

	klog.InfoS("initializing zulip",
		"url", transport.LogURL(server),
		"channel", channel)

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

// SendIncident posts the incident narrative to the configured topic, with
// the application output in a code block after it. Mentions are
// neutralized so log text cannot notify a whole stream.
func (z *Zulip) SendIncident(
	ctx context.Context, m notification.Message,
) error {
	return z.SendMessage(ctx, safetext.RichWithOutput(m,
		notification.MarkdownDialect(safetext.Zulip, "\n"), "\n\n", 0))
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

// validServer accepts an http(s) server URL. Reserved example domains
// (RFC 2606) are rejected so a copied sample config never sends the bot
// API key to a host nobody controls.
func validServer(server string) bool {
	if !transport.ValidEndpoint(server) {
		return false
	}
	parsed, err := url.Parse(strings.TrimSpace(server))
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	for _, reserved := range []string{
		"example.com", "example.net", "example.org", "example",
	} {
		if host == reserved || strings.HasSuffix(host, "."+reserved) {
			return false
		}
	}
	return true
}
