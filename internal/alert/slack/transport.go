package slack

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/ratelimit"

	slackClient "github.com/slack-go/slack"
)

func (s *Slack) sendAPI(
	ctx context.Context,
	msg *slackClient.WebhookMessage,
) error {
	if msg.Text != "" {
		escaped := *msg
		escaped.Text = escapeMrkdwn(msg.Text)
		msg = &escaped
	}
	if s.apiClient != nil {
		return s.sendAPIWithToken(ctx, msg)
	}
	if len(s.channel) > 0 {
		msg.Channel = s.channel
	}
	// Webhook errors get the same redaction and classification as token
	// errors: the webhook URL is a credential and must never reach a log.
	if s.send != nil {
		return wrapSlackRateLimit(s.send(s.webhook, msg))
	}
	return wrapSlackRateLimit(s.sendContext(ctx, s.webhook, msg))
}

func (s *Slack) sendAPIWithToken(
	ctx context.Context,
	msg *slackClient.WebhookMessage,
) error {
	opts := []slackClient.MsgOption{}
	if len(msg.Text) > 0 {
		opts = append(opts, slackClient.MsgOptionText(msg.Text, false))
	}
	if msg.Blocks != nil {
		opts = append(opts, slackClient.MsgOptionBlocks(msg.Blocks.BlockSet...))
	}
	channelID, _, err := s.apiClient.PostMessageContext(
		ctx,
		s.channel,
		opts...,
	)
	if err == nil {
		s.rememberChannelID(channelID)
	}
	return wrapSlackRateLimit(err)
}

// transientSlackErrors are the Slack API error codes that describe the
// service, not the request: a later attempt can succeed. Every other bare
// code (restricted_action, access_denied, missing_scope, ...) is permanent,
// because Slack keeps refusing it and a retry would block the queue.
var transientSlackErrors = map[string]bool{
	"internal_error":      true,
	"fatal_error":         true,
	"service_unavailable": true,
	"request_timeout":     true,
	"ratelimited":         true,
	"rate_limited":        true,
}

// isSlackAPICode reports whether text is a bare Slack error code. slack-go
// surfaces API refusals this way; network and context errors carry spaces
// or punctuation.
func isSlackAPICode(text string) bool {
	if text == "" {
		return false
	}
	for _, r := range text {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}
	return true
}

// wrapSlackRateLimit classifies a Slack API error so retry logic can act on
// it: rate limits carry their Retry-After, Slack API codes are permanent
// unless known to be transient, and anything that is not a Slack code
// (network, context) is left as a transient failure.
func wrapSlackRateLimit(err error) error {
	if err == nil {
		return nil
	}
	err = transport.RedactURLError(err)
	var rle *slackClient.RateLimitedError
	if errors.As(err, &rle) {
		return &ratelimit.Error{
			Provider:   "Slack",
			StatusCode: http.StatusTooManyRequests,
			RetryAfter: rle.RetryAfter,
		}
	}
	var statusErr interface{ HTTPStatusCode() int }
	if errors.As(err, &statusErr) {
		code := statusErr.HTTPStatusCode()
		if code == http.StatusTooManyRequests {
			return &ratelimit.Error{Provider: "Slack", StatusCode: code}
		}
		return transport.ClassifyHTTPStatus(code, err)
	}
	code := strings.TrimSpace(err.Error())
	if isSlackAPICode(code) && !transientSlackErrors[code] {
		return transport.Permanent(err)
	}
	return err
}
