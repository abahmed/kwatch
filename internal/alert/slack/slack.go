package slack

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/message"
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

			for _, chunk := range util.Chunks(logs, chunkSize) {
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
	return wrapSlackRateLimit(err)
}

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
		s.saveThread(key, ts)
		return nil

	case model.ActionUpdate:
		threadTS := s.loadThread(key)
		blocks := buildIncidentUpdateBlocks(inc)
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
	return ts, wrapSlackRateLimit(err)
}

func buildIncidentBlocks(inc *model.Incident, appCfg *config.App) *slackClient.Blocks {
	duration := inc.LastSeen.Sub(inc.FirstSeen).Round(time.Minute)

	blocks := []slackClient.Block{
		markdownSection(fmt.Sprintf("🚨 *%s*", inc.Reason)),
	}

	fields := []*slackClient.TextBlockObject{
		markdownF("*Cluster*\n%s", appCfg.ClusterName),
		markdownF("*Name*\n%s", inc.Name),
	}
	if inc.OwnerKind != "" {
		fields = append(fields, markdownF("*Kind*\n%s", inc.OwnerKind))
	}
	if inc.Namespace != "" {
		fields = append(fields, markdownF("*Namespace*\n%s", inc.Namespace))
	}
	containerName := containerSummary(inc)
	if containerName != "" {
		fields = append(fields, markdownF("*Container*\n%s", containerName))
	}
	if inc.Image != "" {
		fields = append(fields, markdownF("*Image*\n%s", inc.Image))
	}
	fields = append(fields, markdownF("*Reason*\n%s", inc.Reason))
	if inc.NodeName != "" {
		fields = append(fields, markdownF("*Node*\n%s", inc.NodeName))
	}
	if inc.LastContainerState != nil && inc.LastContainerState.ExitCode > 0 {
		fields = append(fields, markdownF("*Exit Code*\n%d", inc.LastContainerState.ExitCode))
	}
	if inc.LastContainerState != nil && inc.LastContainerState.Msg != "" {
		fields = append(fields, markdownF("*Message*\n%s", inc.LastContainerState.Msg))
	}
	if inc.RestartCount > 0 {
		fields = append(fields, markdownF("*Restarts*\n%d", inc.RestartCount))
	}
	fields = append(fields, markdownF("*Count*\n%d", inc.Count))

	resources := make([]string, 0, len(inc.Resources))
	for r := range inc.Resources {
		resources = append(resources, r)
	}
	resourcesStr := strings.Join(resources, ", ")
	if len(resourcesStr) > 200 {
		resourcesStr = resourcesStr[:200] + "..."
	}
	if len(resources) > 0 || inc.PeakResources > 0 {
		peak := ""
		if inc.PeakResources > 0 {
			peak = fmt.Sprintf(" (Peak: %d)", inc.PeakResources)
		}
		fields = append(fields, markdownF("*Resources%s*\n%s", peak, resourcesStr))
	}

	fields = append(fields, markdownF("*Duration*\n%s", duration))

	blocks = append(blocks, slackClient.SectionBlock{
		Type:   "section",
		Fields: fields,
	})

	if inc.Hint != "" {
		blocks = append(blocks, markdownSection("💡 "+inc.Hint))
	}

	if inc.Analysis != "" {
		blocks = append(blocks, markdownSection("*🤖 Likely cause:* "+inc.Analysis))
	}

	if inc.IncludeEvents {
		events := strings.TrimSpace(inc.Events)
		if len(events) > 0 {
			blocks = append(blocks, markdownSection(":mag: *Events*"))
			for _, chunk := range util.Chunks(events, chunkSize) {
				blocks = append(blocks, markdownSectionF("```%s```", chunk))
			}
		}
	}

	if inc.IncludeLogs {
		logs := strings.TrimSpace(inc.Logs)
		if len(logs) > 0 {
			blocks = append(blocks, markdownSection(":memo: *Logs*"))
			for _, chunk := range util.Chunks(logs, chunkSize) {
				blocks = append(blocks, markdownSectionF("```%s```", chunk))
			}
		}
	}

	return &slackClient.Blocks{
		BlockSet: append(blocks, markdownSection(constant.Footer)),
	}
}

func buildIncidentUpdateBlocks(inc *model.Incident) *slackClient.Blocks {
	duration := inc.LastSeen.Sub(inc.FirstSeen).Round(time.Minute)

	header := fmt.Sprintf("🔄 *%s*", inc.Reason)
	if inc.Name != "" {
		header += " — " + inc.Name
	}
	if inc.Namespace != "" {
		header += fmt.Sprintf(" (%s)", inc.Namespace)
	}

	var infoParts []string
	containerName := containerSummary(inc)
	if containerName != "" {
		infoParts = append(infoParts, fmt.Sprintf("Container: %s", containerName))
	}
	if inc.Image != "" {
		infoParts = append(infoParts, fmt.Sprintf("Image: %s", inc.Image))
	}
	if inc.NodeName != "" {
		infoParts = append(infoParts, fmt.Sprintf("Node: %s", inc.NodeName))
	}
	if inc.LastContainerState != nil && inc.LastContainerState.Msg != "" {
		infoParts = append(infoParts, fmt.Sprintf("Message: %s", inc.LastContainerState.Msg))
	}
	if inc.LastContainerState != nil && inc.LastContainerState.ExitCode > 0 {
		infoParts = append(infoParts, fmt.Sprintf("Exit Code: %d", inc.LastContainerState.ExitCode))
	}
	if inc.OwnerKind != "" {
		infoParts = append(infoParts, fmt.Sprintf("Kind: %s", inc.OwnerKind))
	}
	infoParts = append(infoParts, fmt.Sprintf("Count: %d", inc.Count))
	infoParts = append(infoParts, fmt.Sprintf("Duration: %s", duration))
	if inc.RestartCount > 0 {
		infoParts = append(infoParts, fmt.Sprintf("Restarts: %d", inc.RestartCount))
	}
	if inc.PeakResources > 0 {
		infoParts = append(infoParts, fmt.Sprintf("Peak: %d %s", inc.PeakResources, resourcePlural(inc)))
	}

	blocks := []slackClient.Block{
		markdownSection(header + "\n" + strings.Join(infoParts, " · ")),
	}

	if inc.Hint != "" {
		blocks = append(blocks, markdownSection("💡 "+inc.Hint))
	}

	if inc.IncludeEvents {
		events := strings.TrimSpace(inc.Events)
		if len(events) > 0 {
			blocks = append(blocks, markdownSection(":mag: *Events*"))
			for _, chunk := range util.Chunks(events, chunkSize) {
				blocks = append(blocks, markdownSectionF("```%s```", chunk))
			}
		}
	}

	if inc.IncludeLogs {
		logs := strings.TrimSpace(inc.Logs)
		if len(logs) > 0 {
			blocks = append(blocks, markdownSection(":memo: *Logs*"))
			for _, chunk := range util.Chunks(logs, chunkSize) {
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

	header := fmt.Sprintf("✅ *Resolved* — *%s*", inc.Reason)
	if inc.Resource != "" && inc.Name != "" {
		header += fmt.Sprintf(" in %s/%s", inc.Resource, inc.Name)
	} else if inc.Name != "" {
		header += " — " + inc.Name
	}
	if inc.Namespace != "" {
		header += fmt.Sprintf(" (%s)", inc.Namespace)
	}

	var infoParts []string
	infoParts = append(infoParts, fmt.Sprintf("Duration: %s", duration))
	if inc.NodeName != "" {
		infoParts = append(infoParts, fmt.Sprintf("Node: %s", inc.NodeName))
	}
	infoParts = append(infoParts, fmt.Sprintf("Total events: %d", inc.Count))
	if inc.PeakResources > 0 {
		infoParts = append(infoParts, fmt.Sprintf("Peak: %d %s", inc.PeakResources, resourcePlural(inc)))
	}

	text := header + "\n" + strings.Join(infoParts, " · ")

	return &slackClient.Blocks{
		BlockSet: []slackClient.Block{
			markdownSection(text),
		},
	}
}

func formatIncidentText(inc *model.Incident, action model.IncidentAction) string {
	renderer := message.NewSlackRenderer()
	report := message.NewReportBuilder("").Build(inc, action, nil)
	return message.RenderAction(renderer, report)
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
	if inc.ContainerName != "" && inc.ContainerName != "." {
		return inc.ContainerName
	}
	return ""
}

func resourcePlural(inc *model.Incident) string {
	if inc.Resource != "" {
		return inc.Resource + "s"
	}
	return "resources"
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
		Text: markdownF(format, a...),
	}
}
