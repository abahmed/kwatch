package slack

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/ratelimit"

	slackClient "github.com/slack-go/slack"
)

func (s *Slack) sendAPI(msg *slackClient.WebhookMessage) error {
	if s.apiClient != nil {
		return s.sendAPIWithToken(msg)
	}
	if len(s.channel) > 0 {
		msg.Channel = s.channel
	}
	return s.send(s.webhook, msg)
}

func (s *Slack) sendAPIWithToken(msg *slackClient.WebhookMessage) error {
	opts := []slackClient.MsgOption{}
	if len(msg.Text) > 0 {
		opts = append(opts, slackClient.MsgOptionText(msg.Text, false))
	}
	if msg.Blocks != nil {
		opts = append(opts, slackClient.MsgOptionBlocks(msg.Blocks.BlockSet...))
	}
	_, _, err := s.apiClient.PostMessageContext(
		context.Background(),
		s.channel,
		opts...,
	)
	return wrapSlackRateLimit(err)
}

// permanentSlackErrors are Slack API error codes that describe the request or
// the credentials, not the service. Retrying them cannot succeed. The
// slack-go client surfaces these as errors whose text is the bare code.
var permanentSlackErrors = map[string]bool{
	"invalid_blocks":        true,
	"invalid_blocks_format": true,
	"invalid_arguments":     true,
	"invalid_auth":          true,
	"not_authed":            true,
	"token_revoked":         true,
	"token_expired":         true,
	"account_inactive":      true,
	"channel_not_found":     true,
	"not_in_channel":        true,
	"is_archived":           true,
	"msg_too_long":          true,
	"no_text":               true,
	"missing_scope":         true,
}

// wrapSlackRateLimit classifies a Slack API error so retry logic can act on
// it: rate limits carry their Retry-After, permanent rejections are marked so
// they are not retried, everything else is left as a transient failure.
func wrapSlackRateLimit(err error) error {
	if err == nil {
		return nil
	}
	var rle *slackClient.RateLimitedError
	if errors.As(err, &rle) {
		return &ratelimit.Error{
			Provider:   "Slack",
			StatusCode: http.StatusTooManyRequests,
			RetryAfter: rle.RetryAfter,
		}
	}
	if permanentSlackErrors[strings.TrimSpace(err.Error())] {
		return event.Permanent(err)
	}
	return err
}
