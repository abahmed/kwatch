package slack

import (
	"fmt"
	"strings"
	"sync"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/event"

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
