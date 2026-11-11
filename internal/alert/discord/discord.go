package discord

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/ratelimit"

	discordgo "github.com/bwmarrin/discordgo"
	"k8s.io/klog/v2"
)

// maxContent is Discord's message content limit.
const maxContent = 2000

// defaultHost is where webhooks live unless the URL names another host.
const defaultHost = "https://discord.com"

type Discord struct {
	httpClient *http.Client
	// guarded is the client every send goes through (see NewDiscord).
	guarded *http.Client
	sender  transport.Sender
	host    string
	id      string
	token   string
	send    func(webhookID,
		token string,
		wait bool,
		data *discordgo.WebhookParams,
		options ...discordgo.RequestOption) (st *discordgo.Message, err error)

	// threadID posts into a forum or channel thread when the webhook URL
	// carries ?thread_id=.
	threadID   string
	sendThread func(webhookID,
		token string,
		wait bool,
		threadID string,
		data *discordgo.WebhookParams,
		options ...discordgo.RequestOption) (st *discordgo.Message, err error)

	// reference for general app configuration
	clusterName string
	clockSource clock.Clock
}

// parseWebhook extracts the id, token and optional thread id from a
// webhook URL such as https://discord.com/api/webhooks/ID/TOKEN?thread_id=T.
func parseWebhook(webhook string) (string, string, string, bool) {
	parsed, err := url.Parse(webhook)
	if err != nil {
		return "", "", "", false
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 2 || parts[len(parts)-1] == "" ||
		parts[len(parts)-2] == "" {
		return "", "", "", false
	}
	return parts[len(parts)-2], parts[len(parts)-1],
		parsed.Query().Get("thread_id"), true
}

// webhookHost is the scheme and host of the webhook URL, or "" when the
// URL has none (the default host is used then).
func webhookHost(webhook string) string {
	parsed, err := url.Parse(webhook)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

// toWebhookHost sends c's requests for the library's default host to
// host (a webhook on discordapp.com, or a compatible relay), or leaves
// them alone when host is empty or the default.
func toWebhookHost(c *http.Client, host string) *http.Client {
	target, err := url.Parse(host)
	if c == nil || host == "" || err != nil ||
		target.Host == defaultWebhookHost {
		return c
	}
	copied := *c
	base := copied.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	copied.Transport = hostRewrite{base: base, scheme: target.Scheme,
		host: target.Host}
	return &copied
}

// defaultWebhookHost is the host the library builds webhook URLs on.
const defaultWebhookHost = "discord.com"

// hostRewrite points requests for defaultWebhookHost at another host.
type hostRewrite struct {
	base         http.RoundTripper
	scheme, host string
}

func (h hostRewrite) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Host != defaultWebhookHost {
		return h.base.RoundTrip(r)
	}
	r = r.Clone(r.Context())
	r.URL.Scheme, r.URL.Host, r.Host = h.scheme, h.host, h.host
	return h.base.RoundTrip(r)
}

// NewDiscord returns new Discord instance
func NewDiscord(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Discord {
	httpClient := dependencies.HTTPClient
	webhook, ok := config["webhook"].(string)
	if !ok || len(webhook) == 0 {
		klog.InfoS("initializing discord with empty webhook url")
		return nil
	}

	webhookID, webhookToken, threadID, ok := parseWebhook(webhook)
	if !ok {
		klog.InfoS("initializing discord with missing id or token")
		return nil
	}
	klog.InfoS("initializing discord with webhook configured")

	// The guarded client (no redirect, a capped body), sending to the
	// host the webhook names: the library always builds discord.com URLs.
	guarded := toWebhookHost(transport.SDKClient(httpClient),
		webhookHost(webhook))
	sessions := sessionSource{client: guarded}

	return &Discord{
		httpClient:  httpClient,
		guarded:     guarded,
		sender:      transport.NewSender(dependencies),
		host:        webhookHost(webhook),
		id:          webhookID,
		token:       webhookToken,
		send:        sessions.execute,
		threadID:    threadID,
		sendThread:  sessions.executeThread,
		clusterName: clusterName,
		clockSource: clock.Require(dependencies.Clock),
	}
}

// Name returns name of the provider
func (d *Discord) Name() string {
	return "Discord"
}

// Verify checks webhook credentials by issuing a GET to the webhook URL.
func (d *Discord) Verify(ctx context.Context) error {
	_, err := d.sender.Send(ctx, transport.Request{
		Provider: "Discord",
		Method:   http.MethodGet,
		URL:      d.verifyURL(),
	})
	return err
}

// verifyURL is the webhook's own URL: the configured host (discord.com,
// discordapp.com or a compatible server) and the thread it posts into.
func (d *Discord) verifyURL() string {
	host := d.host
	if host == "" {
		host = defaultHost
	}
	target := fmt.Sprintf("%s/api/webhooks/%s/%s",
		host, url.PathEscape(d.id), url.PathEscape(d.token))
	if d.threadID != "" {
		target += "?thread_id=" + url.QueryEscape(d.threadID)
	}
	return target
}

// SendIncident posts the incident as one embed (see incidentEmbed).
// Plain notices and the digest go through SendMessage as content.
func (d *Discord) SendIncident(
	ctx context.Context,
	m notification.Message,
) error {
	klog.V(4).InfoS("sending incident to discord",
		"component", "discord", "conversation", m.Key)
	return d.execute(ctx, &discordgo.WebhookParams{
		AllowedMentions: noMentions(),
		Embeds: []*discordgo.MessageEmbed{
			incidentEmbed(m, d.clusterName, d.clockSource.Now()),
		},
	})
}

// SendMessage sends text using the caller's cancellation context.
func (d *Discord) SendMessage(
	ctx context.Context,
	msg string,
) error {
	return d.execute(ctx, &discordgo.WebhookParams{
		AllowedMentions: noMentions(),
		// The delivery bound is the embed's; plain content has its
		// own, smaller limit.
		Content: notification.Truncate(msg, maxContent),
	})
}

func wrapDiscordRateLimit(err error) error {
	if err == nil {
		return nil
	}
	err = transport.RedactURLError(err)
	var rle *discordgo.RateLimitError
	if errors.As(err, &rle) {
		return &ratelimit.Error{
			Provider:   "Discord",
			StatusCode: http.StatusTooManyRequests,
			RetryAfter: rle.RetryAfter,
		}
	}
	var restErr *discordgo.RESTError
	if errors.As(err, &restErr) && restErr.Response != nil {
		return transport.ClassifyHTTPStatus(
			"Discord", restErr.Response.StatusCode, err)
	}
	return err
}

// noMentions disables @everyone, @here, user and role pings. Messages carry
// workload logs, which must never be able to notify a whole server.
func noMentions() *discordgo.MessageAllowedMentions {
	return &discordgo.MessageAllowedMentions{
		Parse: []discordgo.AllowedMentionType{},
	}
}

// sessionSource makes one library session per send.
type sessionSource struct {
	client *http.Client
}

func (s sessionSource) session() (*discordgo.Session, error) {
	session, err := discordgo.New("")
	if err != nil {
		return nil, err
	}
	if s.client != nil {
		session.Client = s.client
	}
	// A 429 must surface instead of being slept through and retried.
	session.ShouldRetryOnRateLimit = false
	session.MaxRestRetries = 0
	return session, nil
}

// noteRateLimits returns a copy of c that records a 429 and its
// Retry-After header in note, for one send.
func noteRateLimits(
	c *http.Client, note *limitNote, now func() time.Time,
) *http.Client {
	if c == nil {
		return nil
	}
	copied := *c
	base := copied.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	copied.Transport = limitNoting{base: base, note: note, now: now}
	return &copied
}

type limitNoting struct {
	base http.RoundTripper
	note *limitNote
	now  func() time.Time
}

func (l limitNoting) RoundTrip(r *http.Request) (*http.Response, error) {
	response, err := l.base.RoundTrip(r)
	if err != nil || response == nil ||
		response.StatusCode != http.StatusTooManyRequests {
		return response, err
	}
	l.note.limited = true
	l.note.retryAfter = ratelimit.ParseRetryAfterAt(response, l.now())
	return response, err
}
