package discord

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/abahmed/kwatch/internal/alert/safetext"
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
	sender     transport.Sender
	host       string
	id         string
	token      string
	send       func(webhookID,
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

// execute sends through the webhook, inside the configured thread if any.
func (d *Discord) execute(
	data *discordgo.WebhookParams,
	options ...discordgo.RequestOption,
) error {
	if d.threadID != "" && d.sendThread != nil {
		_, err := d.sendThread(
			d.id, d.token, false, d.threadID, data, options...,
		)
		return err
	}
	_, err := d.send(d.id, d.token, false, data, options...)
	return err
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

	discordClient, err := discordgo.New("")
	if err != nil {
		klog.ErrorS(err, "initializing discord client")
		return nil
	}
	discordClient.Client = httpClient
	disableLibraryRetries(discordClient)

	return &Discord{
		httpClient:  httpClient,
		sender:      transport.NewSender(dependencies),
		host:        webhookHost(webhook),
		id:          webhookID,
		token:       webhookToken,
		send:        discordClient.WebhookExecute,
		threadID:    threadID,
		sendThread:  discordClient.WebhookThreadExecute,
		clusterName: clusterName,
		clockSource: clock.Require(dependencies.Clock),
	}
}

// disableLibraryRetries stops discordgo from sleeping through a 429 on its
// own. That sleep ignores the context, so the 429 must surface and be
// handled by delivery's rate limiting instead.
func disableLibraryRetries(s *discordgo.Session) {
	s.ShouldRetryOnRateLimit = false
	s.MaxRestRetries = 0
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

// SendIncident posts the incident narrative as the message content, with
// the workload's last output as a code block. The content is cut to
// Discord's 2000-character limit; bytes never undercount characters.
func (d *Discord) SendIncident(
	ctx context.Context,
	m notification.Message,
) error {
	klog.V(4).InfoS("sending incident to discord",
		"component", "discord", "conversation", m.Key)
	err := d.execute(
		&discordgo.WebhookParams{
			AllowedMentions: noMentions(),
			Content:         incidentContent(m),
		},
		discordgo.WithContext(ctx),
	)
	return wrapDiscordRateLimit(err)
}

// incidentContent is the Note plus the last output, mention-neutralized
// and bounded.
func incidentContent(m notification.Message) string {
	return safetext.RichWithOutput(m,
		notification.MarkdownDialect(nil, "\n"), "\n", maxContent)
}

// SendMessage sends text using the caller's cancellation context.
func (d *Discord) SendMessage(
	ctx context.Context,
	msg string,
) error {
	// send message
	err := d.execute(
		&discordgo.WebhookParams{
			AllowedMentions: noMentions(),
			Content:         msg,
		},
		discordgo.WithContext(ctx),
	)
	return wrapDiscordRateLimit(err)
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
		return transport.ClassifyHTTPStatus(restErr.Response.StatusCode, err)
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
