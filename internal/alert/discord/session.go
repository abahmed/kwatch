package discord

import (
	"context"
	"errors"
	"time"

	discordgo "github.com/bwmarrin/discordgo"

	"github.com/abahmed/kwatch/internal/ratelimit"
)

// The library keeps rate-limit state in its session: after an answer
// that emptied a bucket, or named a global limit, it sleeps before the
// next request, and that sleep ignores the context. A long limit would
// hold a delivery worker past its deadline and past shutdown. Every
// send therefore uses a session of its own, which has no such state;
// delivery's own rate limiting does the waiting.

func (s sessionSource) execute(
	webhookID, token string, wait bool, data *discordgo.WebhookParams,
	options ...discordgo.RequestOption,
) (*discordgo.Message, error) {
	session, err := s.session()
	if err != nil {
		return nil, err
	}
	return session.WebhookExecute(webhookID, token, wait, data, options...)
}

func (s sessionSource) executeThread(
	webhookID, token string, wait bool, threadID string,
	data *discordgo.WebhookParams, options ...discordgo.RequestOption,
) (*discordgo.Message, error) {
	session, err := s.session()
	if err != nil {
		return nil, err
	}
	return session.WebhookThreadExecute(
		webhookID, token, wait, threadID, data, options...)
}

// execute sends through the webhook, inside the configured thread if
// any, and classifies the outcome.
func (d *Discord) execute(
	ctx context.Context, data *discordgo.WebhookParams,
) error {
	note := &limitNote{}
	options := []discordgo.RequestOption{discordgo.WithContext(ctx)}
	if d.guarded != nil {
		options = append(options, discordgo.WithClient(
			noteRateLimits(d.guarded, note, d.clockSource.Now)))
	}
	var err error
	if d.threadID != "" && d.sendThread != nil {
		_, err = d.sendThread(
			d.id, d.token, false, d.threadID, data, options...)
	} else {
		_, err = d.send(d.id, d.token, false, data, options...)
	}
	return note.apply(wrapDiscordRateLimit(err))
}

// limitNote is what the transport saw of a 429 during one send. The
// library reads the wait only from a JSON body: a 429 from a proxy in
// front of Discord (an HTML page) comes back as a plain decoding error
// and its Retry-After header would be lost.
type limitNote struct {
	limited    bool
	retryAfter time.Duration
}

// statusTooManyRequests is HTTP 429.
const statusTooManyRequests = 429

// apply turns the error of a send that was answered 429 into a rate
// limit carrying the header's wait, unless the library already made it
// one with a wait of its own.
func (n *limitNote) apply(err error) error {
	if err == nil || !n.limited {
		return err
	}
	var limited *ratelimit.Error
	if errors.As(err, &limited) {
		if limited.RetryAfter <= 0 {
			limited.RetryAfter = n.retryAfter
		}
		return limited
	}
	return &ratelimit.Error{
		Provider:   "Discord",
		StatusCode: statusTooManyRequests,
		RetryAfter: n.retryAfter,
	}
}
