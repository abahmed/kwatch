package slack

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/ratelimit"

	slackClient "github.com/slack-go/slack"
	"k8s.io/klog/v2"
)

const (
	chunkSize = 2000
)

type Slack struct {
	title   string
	text    string
	channel string
	appCfg  *config.App

	// webhook mode
	webhook string
	send    func(url string, msg *slackClient.WebhookMessage) error

	// token mode
	token     string
	apiClient *slackClient.Client

	// thread support
	threadMap map[string]string
	mu        sync.Mutex

	// maxThreadMapSize bounds the thread map to prevent unbounded growth.
	// When exceeded, new threads are not tracked (updates/resolves still work
	// without threading, just not threaded).
	maxThreadMapSize int

	// compact mode sends single-line messages instead of rich embeds
	compact bool

	// overridable in tests
	postBlocksFn func(
		blocks *slackClient.Blocks, threadTS string,
	) (string, error)
}

// NewSlack returns new Slack instance
func NewSlack(config map[string]interface{}, appCfg *config.App) *Slack {
	title, _ := config["title"].(string)
	text, _ := config["text"].(string)
	compact, _ := config["compact"].(bool)

	// token mode: requires token + channel
	token, hasToken := config["token"].(string)
	channel, hasChannel := config["channel"].(string)
	if hasToken && len(token) > 0 {
		if !hasChannel || len(channel) == 0 {
			klog.InfoS("initializing slack with token but missing channel")
			return nil
		}
		klog.InfoS(
			"initializing slack with token and channel",
			"channel",
			channel,
		)
		return &Slack{
			token:            token,
			channel:          channel,
			title:            title,
			text:             text,
			compact:          compact,
			appCfg:           appCfg,
			apiClient:        slackClient.New(token),
			maxThreadMapSize: 1000,
		}
	}

	// webhook mode: requires webhook
	webhook, ok := config["webhook"].(string)
	if !ok || len(webhook) == 0 {
		klog.InfoS("initializing slack with empty webhook url and no token")
		return nil
	}

	klog.InfoS("initializing slack with webhook url", "webhook", webhook)

	return &Slack{
		webhook:          webhook,
		channel:          channel,
		title:            title,
		text:             text,
		compact:          compact,
		maxThreadMapSize: 1000,
		appCfg:           appCfg,
		send:             slackClient.PostWebhook,
	}
}

// Name returns name of the provider
func (s *Slack) Name() string {
	return "Slack"
}

// Verify checks credentials via Slack auth.test (token mode) or webhook URL.
func (s *Slack) Verify() error {
	if s.apiClient != nil {
		_, err := s.apiClient.AuthTest()
		return err
	}
	if s.webhook == "" {
		return fmt.Errorf("slack: no webhook or token configured")
	}
	return nil
}

// SendEvent sends event to the provider
func (s *Slack) SendEvent(ev *event.Event) error {
	klog.InfoS("sending to slack event", "event", ev)

	// compact mode: single-line text message
	if s.compact {
		text := fmt.Sprintf(
			"K8s Alert: %s - %s (%s)",
			ev.PodName, ev.Reason, ev.Namespace,
		)
		return s.sendAPI(&slackClient.WebhookMessage{
			Text: text,
		})
	}

	// use custom title if it's provided, otherwise use default
	title := s.title
	if len(title) == 0 {
		title = constant.DefaultTitle
	}

	// use custom text if it's provided, otherwise use default
	text := s.text
	if len(text) == 0 {
		text = constant.DefaultText
	}

	blocks := []slackClient.Block{
		markdownSection(title),
		plainSection(text),
		slackClient.SectionBlock{
			Type: "section",
			Fields: []*slackClient.TextBlockObject{
				markdownF("*Cluster*\n%s", s.appCfg.ClusterName),
				markdownF("*Name*\n%s", ev.PodName),
				markdownF("*Container*\n%s", ev.ContainerName),
				markdownF("*Namespace*\n%s", ev.Namespace),
				markdownF("*Node*\n%s", ev.NodeName),
				markdownF("*Reason*\n%s", ev.Reason),
			},
		},
	}

	// add events part if it exists
	if ev.IncludeEvents {
		events := strings.TrimSpace(ev.Events)
		if len(events) > 0 {
			blocks = append(blocks,
				markdownSection(":mag: *Events*"))

			for _, chunk := range util.Chunks(events, chunkSize) {
				blocks = append(blocks,
					markdownSection("```"+chunk+"```"))
			}
		}
	}

	// add logs part if it exists
	if ev.IncludeLogs {
		logs := strings.TrimSpace(ev.Logs)
		if len(logs) > 0 {
			blocks = append(blocks,
				markdownSection(":memo: *Logs*"))

			for _, chunk := range util.Chunks(logs, chunkSize) {
				blocks = append(blocks,
					markdownSection("```"+chunk+"```"))
			}
		}
	}

	// send message
	return s.sendAPI(&slackClient.WebhookMessage{
		Blocks: &slackClient.Blocks{
			BlockSet: append(blocks, markdownSection(constant.Footer)),
		},
	})
}

// SendMessage sends text message to the provider
func (s *Slack) SendMessage(msg string) error {
	return s.sendAPI(&slackClient.WebhookMessage{
		Text: msg,
	})
}

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

// SendIncident implements alert.ThreadProvider.
// In token mode it posts rich blocks and threads updates.
// In webhook mode it falls back to SendMessage.
func (s *Slack) SendIncident(
	inc *model.Incident,
	action model.IncidentAction,
) error {
	if action == model.ActionSkip {
		return nil
	}
	if s.compact {
		return s.SendMessage(formatIncidentText(inc, action))
	}
	if s.postBlocksFn != nil || s.apiClient != nil {
		return s.sendIncidentWithToken(inc, action, nil)
	}
	return s.SendMessage(formatIncidentText(inc, action))
}

// SendIncidentWithInsight implements alert.InsightThreadProvider: the same as
// SendIncident, with the diagnosis rendered as its own block.
func (s *Slack) SendIncidentWithInsight(
	inc *model.Incident,
	action model.IncidentAction,
	ins *insight.Insight,
) error {
	if action == model.ActionSkip {
		return nil
	}
	if s.compact {
		return s.SendMessage(formatIncidentText(inc, action))
	}
	if s.postBlocksFn != nil || s.apiClient != nil {
		return s.sendIncidentWithToken(inc, action, ins)
	}
	return s.SendMessage(formatIncidentText(inc, action))
}

func (s *Slack) sendIncidentWithToken(
	inc *model.Incident,
	action model.IncidentAction,
	ins *insight.Insight,
) error {
	key := string(inc.Key)

	post := s.postBlocks
	if s.postBlocksFn != nil {
		post = s.postBlocksFn
	}

	switch action {
	case model.ActionCreate:
		blocks := buildIncidentBlocksWithInsight(inc, s.appCfg, ins)
		ts, err := post(blocks, "")
		if err != nil {
			return err
		}
		s.saveThread(key, ts)
		return nil

	case model.ActionUpdate:
		threadTS := s.loadThread(key)
		blocks := buildIncidentUpdateBlocksWithInsight(inc, ins)
		_, err := post(blocks, threadTS)
		return err

	case model.ActionResolved:
		threadTS := s.popThread(key)
		blocks := buildIncidentResolvedBlocks(inc)
		_, err := post(blocks, threadTS)
		return err
	}

	return nil
}

func (s *Slack) saveThread(key, ts string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.threadMap == nil {
		s.threadMap = make(map[string]string)
	}
	if s.maxThreadMapSize <= 0 || len(s.threadMap) < s.maxThreadMapSize {
		s.threadMap[key] = ts
	}
}

func (s *Slack) loadThread(key string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	ts, ok := s.threadMap[key]
	if !ok {
		return ""
	}
	return ts
}

func (s *Slack) popThread(key string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	ts, _ := s.threadMap[key]
	delete(s.threadMap, key)
	return ts
}

func (s *Slack) postBlocks(
	blocks *slackClient.Blocks,
	threadTS string,
) (string, error) {
	opts := []slackClient.MsgOption{
		slackClient.MsgOptionBlocks(blocks.BlockSet...),
		slackClient.MsgOptionAsUser(true),
	}
	if threadTS != "" {
		opts = append(opts, slackClient.MsgOptionTS(threadTS))
	}
	_, ts, err := s.apiClient.PostMessageContext(
		context.Background(),
		s.channel,
		opts...,
	)
	return ts, wrapSlackRateLimit(err)
}
