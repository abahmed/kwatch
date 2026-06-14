package slack

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"

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

	// compact mode sends single-line messages instead of rich embeds
	compact bool

	// overridable in tests
	postBlocksFn func(blocks *slackClient.Blocks, threadTS string) (string, error)
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
		klog.InfoS("initializing slack with token and channel", "channel", channel)
		return &Slack{
			token:     token,
			channel:   channel,
			title:     title,
			text:      text,
			compact:   compact,
			appCfg:    appCfg,
			apiClient: slackClient.New(token),
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
		webhook: webhook,
		channel: channel,
		title:   title,
		text:    text,
		compact: compact,
		appCfg:  appCfg,
		send:    slackClient.PostWebhook,
	}
}

// Name returns name of the provider
func (s *Slack) Name() string {
	return "Slack"
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

			for _, chunk := range chunks(events, chunkSize) {
				blocks = append(blocks,
					markdownSectionF("```%s```", chunk))
			}
		}
	}

	// add logs part if it exists
	if ev.IncludeLogs {
		logs := strings.TrimSpace(ev.Logs)
		if len(logs) > 0 {
			blocks = append(blocks,
				markdownSection(":memo: *Logs*"))

			for _, chunk := range chunks(logs, chunkSize) {
				blocks = append(blocks,
					markdownSectionF("```%s```", chunk))
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
	return err
}

// SendIncident implements alert.ThreadProvider.
// In token mode it posts rich blocks and threads updates.
// In webhook mode it falls back to SendMessage.
func (s *Slack) SendIncident(inc *model.Incident, action model.IncidentAction) error {
	if action == model.ActionSkip {
		return nil
	}
	if s.compact {
		return s.SendMessage(formatIncidentText(inc, action))
	}
	if s.postBlocksFn != nil || s.apiClient != nil {
		return s.sendIncidentWithToken(inc, action)
	}
	return s.SendMessage(formatIncidentText(inc, action))
}

func (s *Slack) sendIncidentWithToken(inc *model.Incident, action model.IncidentAction) error {
	key := inc.Key

	post := s.postBlocks
	if s.postBlocksFn != nil {
		post = s.postBlocksFn
	}

	switch action {
	case model.ActionCreate:
		blocks := buildIncidentBlocks(inc, s.appCfg)
		ts, err := post(blocks, "")
		if err != nil {
			return err
		}
		s.mu.Lock()
		if s.threadMap == nil {
			s.threadMap = make(map[string]string)
		}
		s.threadMap[key] = ts
		s.mu.Unlock()
		return nil

	case model.ActionUpdate:
		s.mu.Lock()
		threadTS, ok := s.threadMap[key]
		s.mu.Unlock()
		if !ok {
			threadTS = ""
		}
		blocks := buildIncidentUpdateBlocks(inc)
		_, err := post(blocks, threadTS)
		return err

	case model.ActionResolved:
		s.mu.Lock()
		threadTS, _ := s.threadMap[key]
		delete(s.threadMap, key)
		s.mu.Unlock()
		blocks := buildIncidentResolvedBlocks(inc)
		_, err := post(blocks, threadTS)
		return err
	}

	return nil
}

func (s *Slack) postBlocks(blocks *slackClient.Blocks, threadTS string) (string, error) {
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
	return ts, err
}

func buildIncidentBlocks(inc *model.Incident, appCfg *config.App) *slackClient.Blocks {
	resources := make([]string, 0, len(inc.Resources))
	for r := range inc.Resources {
		resources = append(resources, r)
	}
	resourcesStr := strings.Join(resources, ", ")
	if len(resourcesStr) > 200 {
		resourcesStr = resourcesStr[:200] + "..."
	}
	duration := inc.LastSeen.Sub(inc.FirstSeen).Round(time.Minute)
	hint := inc.Hint
	if hint != "" {
		hint = "\n💡 " + hint
	}

	title := "🚨 Incident"
	text := constant.DefaultText

	blocks := []slackClient.Block{
		markdownSection(title),
		plainSection(text),
		slackClient.SectionBlock{
			Type: "section",
			Fields: []*slackClient.TextBlockObject{
				markdownF("*Cluster*\n%s", appCfg.ClusterName),
				markdownF("*Name*\n%s", inc.Name),
				markdownF("*Kind*\n%s", inc.OwnerKind),
				markdownF("*Namespace*\n%s", inc.Namespace),
				markdownF("*Container*\n%s", containerSummary(inc)),
				markdownF("*Reason*\n%s", inc.Reason),
				markdownF("*Restarts*\n%d", inc.RestartCount),
				markdownF("*Count*\n%d", inc.Count),
				markdownF("*Resources (Peak: %d)*\n%s", inc.PeakResources, resourcesStr),
				markdownF("*Duration*\n%s", duration),
			},
		},
	}

	if hint != "" {
		blocks = append(blocks, markdownSection(hint))
	}

	if inc.IncludeEvents {
		events := strings.TrimSpace(inc.Events)
		if len(events) > 0 {
			blocks = append(blocks, markdownSection(":mag: *Events*"))
			for _, chunk := range chunks(events, chunkSize) {
				blocks = append(blocks, markdownSectionF("```%s```", chunk))
			}
		}
	}

	if inc.IncludeLogs {
		logs := strings.TrimSpace(inc.Logs)
		if len(logs) > 0 {
			blocks = append(blocks, markdownSection(":memo: *Logs*"))
			for _, chunk := range chunks(logs, chunkSize) {
				blocks = append(blocks, markdownSectionF("```%s```", chunk))
			}
		}
	}

	return &slackClient.Blocks{
		BlockSet: append(blocks, markdownSection(constant.Footer)),
	}
}

func buildIncidentUpdateBlocks(inc *model.Incident) *slackClient.Blocks {
	resources := make([]string, 0, len(inc.Resources))
	for r := range inc.Resources {
		resources = append(resources, r)
	}
	resourcesStr := strings.Join(resources, ", ")
	if len(resourcesStr) > 200 {
		resourcesStr = resourcesStr[:200] + "..."
	}
	duration := inc.LastSeen.Sub(inc.FirstSeen).Round(time.Minute)

	text := fmt.Sprintf(
		"🔄 Update — Container: %s | Count: %d | Restarts: %d | Resources: %s | Duration: %s",
		containerSummary(inc), inc.Count, inc.RestartCount, resourcesStr, duration,
	)

	blocks := []slackClient.Block{
		markdownSection(text),
	}

	if inc.IncludeEvents {
		events := strings.TrimSpace(inc.Events)
		if len(events) > 0 {
			blocks = append(blocks, markdownSection(":mag: *Events*"))
			for _, chunk := range chunks(events, chunkSize) {
				blocks = append(blocks, markdownSectionF("```%s```", chunk))
			}
		}
	}

	if inc.IncludeLogs {
		logs := strings.TrimSpace(inc.Logs)
		if len(logs) > 0 {
			blocks = append(blocks, markdownSection(":memo: *Logs*"))
			for _, chunk := range chunks(logs, chunkSize) {
				blocks = append(blocks, markdownSectionF("```%s```", chunk))
			}
		}
	}

	return &slackClient.Blocks{
		BlockSet: blocks,
	}
}

func buildIncidentResolvedBlocks(inc *model.Incident) *slackClient.Blocks {
	duration := inc.LastSeen.Sub(inc.FirstSeen).Round(time.Minute)

	text := fmt.Sprintf(
		"✅ *Resolved* — Container: %s\nDuration: %s | Total events: %d | Peak resources: %d",
		containerSummary(inc), duration, inc.Count, inc.PeakResources,
	)

	return &slackClient.Blocks{
		BlockSet: []slackClient.Block{
			markdownSection(text),
		},
	}
}

func formatIncidentText(inc *model.Incident, action model.IncidentAction) string {
	switch action {
	case model.ActionCreate:
		duration := inc.LastSeen.Sub(inc.FirstSeen).Round(time.Minute)
		text := fmt.Sprintf(
			"🚨 Incident: %s (%s)\nNamespace: %s\nContainer: %s\nReason: %s\nRestarts: %d\nHint: %s\nPeak: %d resource(s)\nCount: %d\nDuration: %s",
			inc.Name, inc.OwnerKind, inc.Namespace, containerSummary(inc),
			inc.Reason, inc.RestartCount, inc.Hint,
			inc.PeakResources, inc.Count, duration,
		)
		if inc.IncludeEvents {
			if ev := strings.TrimSpace(inc.Events); len(ev) > 0 {
				text += "\n\nEvents:\n" + ev
			}
		}
		if inc.IncludeLogs {
			if logs := strings.TrimSpace(inc.Logs); len(logs) > 0 {
				text += "\n\nLogs:\n" + logs
			}
		}
		return text
	case model.ActionUpdate:
		duration := inc.LastSeen.Sub(inc.FirstSeen).Round(time.Minute)
		text := fmt.Sprintf(
		"🔄 Update: %s | Container: %s | Count: %d | Duration: %s | Peak: %d",
		inc.Name, containerSummary(inc), inc.Count, duration, inc.PeakResources,
		)
		if inc.IncludeEvents {
			if ev := strings.TrimSpace(inc.Events); len(ev) > 0 {
				text += "\n\nEvents:\n" + ev
			}
		}
		if inc.IncludeLogs {
			if logs := strings.TrimSpace(inc.Logs); len(logs) > 0 {
				text += "\n\nLogs:\n" + logs
			}
		}
		return text
	case model.ActionResolved:
		duration := inc.LastSeen.Sub(inc.FirstSeen).Round(time.Minute)
		return fmt.Sprintf(
		"✅ Resolved: %s | Container: %s | Duration: %s | Total events: %d | Peak resources: %d",
		inc.Name, containerSummary(inc), duration, inc.Count, inc.PeakResources,
		)
	default:
		return ""
	}
}

func containerSummary(inc *model.Incident) string {
	if len(inc.Containers) > 1 {
		names := make([]string, 0, len(inc.Containers))
		for c := range inc.Containers {
			names = append(names, c)
		}
		sort.Strings(names)
		return strings.Join(names, ", ")
	}
	return inc.ContainerName
}

func chunks(s string, chunkSize int) []string {
	if chunkSize >= len(s) {
		return []string{s}
	}
	var chunks []string = make([]string, 0, (len(s)-1)/chunkSize+1)
	currentLen := 0
	currentStart := 0
	for i := range s {
		if currentLen == chunkSize {
			chunks = append(chunks, s[currentStart:i])
			currentLen = 0
			currentStart = i
		}
		currentLen++
	}
	chunks = append(chunks, s[currentStart:])
	return chunks
}

func plainSection(txt string) slackClient.SectionBlock {
	return slackClient.SectionBlock{
		Type: "section",
		Text: slackClient.NewTextBlockObject(
			slackClient.PlainTextType,
			txt,
			true,
			false),
	}
}

func markdownSection(txt string) slackClient.SectionBlock {
	return slackClient.SectionBlock{
		Type: "section",
		Text: slackClient.NewTextBlockObject(
			slackClient.MarkdownType,
			txt,
			false,
			true),
	}
}

func markdownF(format string, a ...interface{}) *slackClient.TextBlockObject {
	return slackClient.NewTextBlockObject(
		slackClient.MarkdownType,
		fmt.Sprintf(format, a...),
		false,
		true)
}

func markdownSectionF(
	format string, a ...interface{}) slackClient.SectionBlock {
	return slackClient.SectionBlock{
		Type: "section",
		Text: markdownF(format, a...),
	}
}
