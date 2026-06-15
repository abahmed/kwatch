package alert

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"text/template"
	"time"
	"unicode/utf8"

	"github.com/abahmed/kwatch/internal/alert/dingtalk"
	"github.com/abahmed/kwatch/internal/alert/discord"
	"github.com/abahmed/kwatch/internal/alert/email"
	"github.com/abahmed/kwatch/internal/alert/feishu"
	"github.com/abahmed/kwatch/internal/alert/googlechat"
	"github.com/abahmed/kwatch/internal/alert/matrix"
	"github.com/abahmed/kwatch/internal/alert/mattermost"
	"github.com/abahmed/kwatch/internal/alert/opsgenie"
	"github.com/abahmed/kwatch/internal/alert/pagerduty"
	"github.com/abahmed/kwatch/internal/alert/rocketchat"
	"github.com/abahmed/kwatch/internal/alert/slack"
	"github.com/abahmed/kwatch/internal/alert/teams"
	"github.com/abahmed/kwatch/internal/alert/telegram"
	"github.com/abahmed/kwatch/internal/alert/webhook"
	"github.com/abahmed/kwatch/internal/alert/zenduty"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/model"
	"k8s.io/klog/v2"
)

type deliverJob struct {
	inc    *model.Incident
	action model.IncidentAction
}

type DeadLetterEntry struct {
	Provider  string               `json:"provider"`
	Key       string               `json:"key"`
	Action    model.IncidentAction `json:"action"`
	Error     string               `json:"error"`
	Timestamp time.Time            `json:"timestamp"`
}

const channelCap = 256
const dlqCap = 100

const defaultMaxBackoff = 30 * time.Second

type providerEntry struct {
	provider      Provider
	routes        []config.AlertRoute
	maxAttempts   int
	retryDelay    time.Duration
	maxBackoff    time.Duration
	fallback      *providerEntry
	fallbackNamed string // resolved in second pass
	templates     map[string]*template.Template
	maxBytes      int // 0 = no limit (FIX-5)
	ch            chan deliverJob
}

type AlertManager struct {
	entries     []providerEntry
	silences    []silenceMatcher
	maxLogLines int
	templates   map[string]*template.Template
	started     bool
	wg          sync.WaitGroup
	dlqMu       sync.Mutex
	dlqRing     [dlqCap]DeadLetterEntry
	dlqHead     int
}

func (a *AlertManager) SetMaxLogLines(n int) {
	if n > 0 {
		a.maxLogLines = n
	}
}

func (a *AlertManager) SetTemplates(tpl map[string]string) {
	if len(tpl) == 0 {
		a.templates = nil
		return
	}
	a.templates = make(map[string]*template.Template, len(tpl))
	for reason, raw := range tpl {
		t, err := template.New(reason).Option("missingkey=zero").Parse(raw)
		if err != nil {
			klog.ErrorS(err, "invalid template, skipping", "reason", reason)
			continue
		}
		a.templates[strings.ToLower(reason)] = t
	}
}

type silenceMatcher struct {
	namespaces     []string
	reasons        []string
	podPattern     []*regexp.Regexp
	containerNames []string
	logPatterns    []*regexp.Regexp
	containerMsgs  []string
	nodeReasons    []string
	nodeMessages   []string
}

// Provider interface
type Provider interface {
	Name() string
	SendEvent(*event.Event) error
	SendMessage(string) error
}

// VerifiableProvider is an optional interface for providers that support
// credential pre-flight verification (kwatch lint --check).
type VerifiableProvider interface {
	Verify() error
}

func extractRoutes(cfg map[string]interface{}) []config.AlertRoute {
	if r, ok := cfg["routes"]; ok {
		if routes, ok := r.([]interface{}); ok {
			out := make([]config.AlertRoute, 0, len(routes))
			for _, ri := range routes {
				if rm, ok := ri.(map[string]interface{}); ok {
					route := config.AlertRoute{}
					if ns, ok := rm["namespaces"]; ok {
						for _, n := range ns.([]interface{}) {
							route.Namespaces = append(route.Namespaces, fmt.Sprint(n))
						}
					}
					if sev, ok := rm["severities"]; ok {
						for _, s := range sev.([]interface{}) {
							route.Severities = append(route.Severities, fmt.Sprint(s))
						}
					}
					if rea, ok := rm["reasons"]; ok {
						for _, r := range rea.([]interface{}) {
							route.Reasons = append(route.Reasons, fmt.Sprint(r))
						}
					}
					if len(route.Namespaces) > 0 || len(route.Severities) > 0 || len(route.Reasons) > 0 {
						out = append(out, route)
					}
				}
			}
			return out
		}
	}
	return nil
}

func extractTemplates(cfg map[string]interface{}) map[string]*template.Template {
	if raw, ok := cfg["templates"]; ok {
		if tpl, ok := raw.(map[string]interface{}); ok {
			out := make(map[string]*template.Template, len(tpl))
			for reason, rawBody := range tpl {
				if body, ok := rawBody.(string); ok {
					t, err := template.New(reason).Option("missingkey=zero").Parse(body)
					if err != nil {
						klog.ErrorS(err, "invalid provider template, skipping", "reason", reason)
						continue
					}
					out[strings.ToLower(reason)] = t
				}
			}
			return out
		}
	}
	return nil
}

func extractRetry(cfg map[string]interface{}) (maxAttempts int, delay, maxBackoff time.Duration) {
	maxAttempts = 1
	delay = time.Second
	maxBackoff = defaultMaxBackoff
	if r, ok := cfg["retry"]; ok {
		if rm, ok := r.(map[string]interface{}); ok {
			if a, ok := rm["maxAttempts"]; ok {
				if f, ok := a.(float64); ok {
					maxAttempts = int(f)
				}
			}
			if d, ok := rm["delay"]; ok {
				if s, ok := d.(string); ok {
					if parsed, err := time.ParseDuration(s); err == nil {
						delay = parsed
					}
				}
			}
			if b, ok := rm["maxBackoff"]; ok {
				if s, ok := b.(string); ok {
					if parsed, err := time.ParseDuration(s); err == nil {
						maxBackoff = parsed
					}
				}
			}
		}
	}
	return
}

// ProviderNames returns the set of known alert provider names.
func ProviderNames() []string {
	names := make([]string, 0, len(config.KnownProviders))
	for n := range config.KnownProviders {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Init initializes AlertManager with provided config.
// Safe to call multiple times: shuts down existing workers before re-init.
func (a *AlertManager) Init(
	alertCfg map[string]map[string]interface{},
	appCfg *config.App,
) {
	if a.started {
		a.shutdown()
	}
	a.entries = make([]providerEntry, 0)
	a.silences = nil

	entries := make([]providerEntry, 0, len(alertCfg))
	for k, v := range alertCfg {
		lowerCaseKey := strings.ToLower(k)
		var pvdr Provider = nil
		if lowerCaseKey == "slack" {
			pvdr = slack.NewSlack(v, appCfg)
		} else if lowerCaseKey == "pagerduty" {
			pvdr = pagerduty.NewPagerDuty(v, appCfg)
		} else if lowerCaseKey == "discord" {
			pvdr = discord.NewDiscord(v, appCfg)
		} else if lowerCaseKey == "telegram" {
			pvdr = telegram.NewTelegram(v, appCfg)
		} else if lowerCaseKey == "teams" {
			pvdr = teams.NewTeams(v, appCfg)
		} else if lowerCaseKey == "email" {
			pvdr = email.NewEmail(v, appCfg)
		} else if lowerCaseKey == "rocketchat" {
			pvdr = rocketchat.NewRocketChat(v, appCfg)
		} else if lowerCaseKey == "mattermost" {
			pvdr = mattermost.NewMattermost(v, appCfg)
		} else if lowerCaseKey == "opsgenie" {
			pvdr = opsgenie.NewOpsgenie(v, appCfg)
		} else if lowerCaseKey == "matrix" {
			pvdr = matrix.NewMatrix(v, appCfg)
		} else if lowerCaseKey == "dingtalk" {
			pvdr = dingtalk.NewDingTalk(v, appCfg)
		} else if lowerCaseKey == "feishu" {
			pvdr = feishu.NewFeiShu(v, appCfg)
		} else if lowerCaseKey == "webhook" {
			pvdr = webhook.NewWebhook(v, appCfg)
		} else if lowerCaseKey == "zenduty" {
			pvdr = zenduty.NewZenduty(v, appCfg)
		} else if lowerCaseKey == "googlechat" {
			pvdr = googlechat.NewGoogleChat(v, appCfg)
		}

		if pvdr == nil {
			if config.KnownProviders[lowerCaseKey] {
				klog.InfoS("alert provider has missing or invalid credentials, skipping", "name", k)
			} else {
				klog.InfoS("unknown alert provider, skipping", "name", k)
			}
			continue
		}
		if !reflect.ValueOf(pvdr).IsNil() {
			maxAttempts, retryDelay, maxBackoff := extractRetry(v)
			fbName := ""
			if raw, ok := v["fallback"]; ok {
				fbName, _ = raw.(string)
			}
			entries = append(entries, providerEntry{
				provider:      pvdr,
				routes:        extractRoutes(v),
				maxAttempts:   maxAttempts,
				retryDelay:    retryDelay,
				maxBackoff:    maxBackoff,
				fallback:      nil,
				fallbackNamed: fbName,
				templates:     extractTemplates(v),
				maxBytes:      defaultMaxBytes(pvdr.Name()),
				ch:            make(chan deliverJob, channelCap),
			})
		}
	}
	// second pass: resolve fallback names to pointers
	for i := range entries {
		if entries[i].fallbackNamed != "" {
			for j := range entries {
				if strings.EqualFold(entries[j].provider.Name(), entries[i].fallbackNamed) {
					entries[i].fallback = &entries[j]
					break
				}
			}
			if entries[i].fallback == nil {
				klog.InfoS("fallback provider not found, skipping", "provider", entries[i].provider.Name(), "fallback", entries[i].fallbackNamed)
			}
			entries[i].fallbackNamed = ""
		}
	}
	a.entries = entries
}

// SetSilences configures silence rules on the alert manager.
// Must be called after Init.
func (a *AlertManager) SetSilences(rules []config.SilenceRule) {
	a.silences = make([]silenceMatcher, 0, len(rules))
	for _, sr := range rules {
		sm := silenceMatcher{
			namespaces:     sr.Namespaces,
			reasons:        sr.Reasons,
			containerNames: sr.ContainerNames,
			containerMsgs:  sr.ContainerMessages,
			nodeReasons:    sr.NodeReasons,
			nodeMessages:   sr.NodeMessages,
		}
		for _, p := range sr.PodNamePatterns {
			if re, err := regexp.Compile(p); err == nil {
				sm.podPattern = append(sm.podPattern, re)
			} else {
				klog.ErrorS(err, "invalid silence pod name pattern", "pattern", p)
			}
		}
		for _, p := range sr.LogPatterns {
			if re, err := regexp.Compile(p); err == nil {
				sm.logPatterns = append(sm.logPatterns, re)
			} else {
				klog.ErrorS(err, "invalid silence log pattern", "pattern", p)
			}
		}
		a.silences = append(a.silences, sm)
	}
}

func (a *AlertManager) isSilenced(inc *model.Incident) bool {
	for _, sm := range a.silences {
		if matchesSilence(sm, inc) {
			return true
		}
	}
	return false
}

func matchesSilence(sm silenceMatcher, inc *model.Incident) bool {
	if len(sm.namespaces) > 0 {
		found := false
		for _, ns := range sm.namespaces {
			if ns == inc.Namespace {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if len(sm.reasons) > 0 {
		found := false
		for _, r := range sm.reasons {
			if r == inc.Reason {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if len(sm.podPattern) > 0 {
		found := false
		for _, re := range sm.podPattern {
			if re.MatchString(inc.Name) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if len(sm.containerNames) > 0 {
		found := false
		for _, cn := range sm.containerNames {
			if cn == inc.ContainerName || inc.Containers[cn] {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if len(sm.logPatterns) > 0 {
		if inc.Logs == "" {
			return false
		}
		found := false
		for _, re := range sm.logPatterns {
			if re.MatchString(inc.Logs) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if len(sm.containerMsgs) > 0 {
		if inc.LastContainerState == nil || inc.LastContainerState.Msg == "" {
			return false
		}
		found := false
		for _, m := range sm.containerMsgs {
			if strings.Contains(inc.LastContainerState.Msg, m) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if len(sm.nodeReasons) > 0 {
		found := false
		for _, r := range sm.nodeReasons {
			if r == inc.Reason {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if len(sm.nodeMessages) > 0 {
		if inc.Hint == "" {
			return false
		}
		found := false
		for _, m := range sm.nodeMessages {
			if strings.Contains(inc.Hint, m) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func matchesRoute(route config.AlertRoute, inc *model.Incident) bool {
	if len(route.Namespaces) > 0 {
		found := false
		for _, ns := range route.Namespaces {
			if ns == inc.Namespace {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if len(route.Severities) > 0 {
		found := false
		for _, s := range route.Severities {
			if s == inc.Severity {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if len(route.Reasons) > 0 {
		found := false
		for _, r := range route.Reasons {
			if r == inc.Reason {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// shouldDeliver checks whether an incident should be delivered to a provider.
// If the provider has no routes defined, all incidents are delivered.
func shouldDeliver(routes []config.AlertRoute, inc *model.Incident) bool {
	if len(routes) == 0 {
		return true
	}
	for _, route := range routes {
		if matchesRoute(route, inc) {
			return true
		}
	}
	return false
}

func backoffFor(attempt int, baseDelay, maxBackoff time.Duration) time.Duration {
	d := baseDelay * time.Duration(1<<(attempt-1))
	if maxBackoff > 0 && d > maxBackoff {
		d = maxBackoff
	}
	if d < baseDelay {
		d = baseDelay
	}
	return d
}

func sendWithRetry(sendFn func() error, maxAttempts int, delay, maxBackoff time.Duration, providerName string) error {
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := sendFn(); err != nil {
			lastErr = err
			if attempt < maxAttempts {
				sleepDur := delay
				if maxBackoff > 0 {
					sleepDur = backoffFor(attempt, delay, maxBackoff)
				}
				klog.V(4).InfoS("retrying provider delivery",
					"provider", providerName,
					"attempt", attempt,
					"maxAttempts", maxAttempts,
					"backoff", sleepDur)
				time.Sleep(sleepDur)
			}
			continue
		}
		return nil
	}
	klog.ErrorS(lastErr, "failed to deliver after retries",
		"provider", providerName,
		"maxAttempts", maxAttempts)
	return lastErr
}

// VerifyAll runs credential pre-flight on all providers that support it.
// Returns a map of provider name → error (nil = verified OK).
func (a *AlertManager) VerifyAll() map[string]error {
	result := make(map[string]error)
	for _, entry := range a.entries {
		if v, ok := entry.provider.(VerifiableProvider); ok {
			result[entry.provider.Name()] = v.Verify()
		} else {
			result[entry.provider.Name()] = nil // no verifier = skip
		}
	}
	return result
}

// Notify sends string msg to all providers
func (a *AlertManager) Notify(msg string) {
	klog.InfoS("sending message", "msg", msg)

	for _, entry := range a.entries {
		p := entry.provider
		truncMsg := truncateMsg(msg, entry.maxBytes)
		if err := sendWithRetry(func() error {
			return p.SendMessage(truncMsg)
		}, entry.maxAttempts, entry.retryDelay, entry.maxBackoff, p.Name()); err != nil && entry.fallback != nil {
			entry.fallback.provider.SendMessage("[fallback — primary " + p.Name() + " failed] " + truncMsg)
		}
	}
}

// NotifyEvent sends event to all providers
func (a *AlertManager) NotifyEvent(event event.Event) {
	klog.InfoS("sending event", "event", event)

	for _, entry := range a.entries {
		p := entry.provider
		if err := sendWithRetry(func() error {
			return p.SendEvent(&event)
		}, entry.maxAttempts, entry.retryDelay, entry.maxBackoff, p.Name()); err != nil && entry.fallback != nil {
			entry.fallback.provider.SendMessage("[fallback — primary " + p.Name() + " failed] " + event.Reason + " in " + event.Namespace + "/" + event.PodName)
		}
	}
}

// ThreadProvider is an optional interface for providers that support
// incident-aware messaging (e.g., Slack threads).
type ThreadProvider interface {
	SendIncident(inc *model.Incident, action model.IncidentAction) error
}

// NotifyIncident enqueues an incident for delivery to all providers.
// When Start has been called, delivery is asynchronous via per-provider
// buffered channels (non-blocking; drops oldest on full).
// Before Start, delivery is synchronous (deliverAllSync).
func (a *AlertManager) NotifyIncident(inc *model.Incident, action model.IncidentAction) {
	if action == model.ActionSkip {
		return
	}

	if a.isSilenced(inc) {
		klog.V(4).InfoS("incident suppressed by silence rule",
			"key", inc.Key, "id", inc.ID, "reason", inc.Reason, "namespace", inc.Namespace)
		return
	}

	klog.InfoS("sending incident", "action", action, "key", inc.Key, "id", inc.ID, "count", inc.Count)

	if !a.started {
		a.deliverAllSync(inc, action)
		return
	}

	snap := inc.Clone()
	job := deliverJob{inc: snap, action: action}
	for _, entry := range a.entries {
		select {
		case entry.ch <- job:
		default:
			<-entry.ch
			metrics.Default.NotificationsDropped.Add(1)
			select {
			case entry.ch <- job:
			default:
			}
		}
	}
}

// Start launches a worker goroutine for each provider that processes
// queued deliveries. Workers drain and stop when ctx is cancelled.
func (a *AlertManager) Start(ctx context.Context) {
	a.started = true
	for i := range a.entries {
		entry := &a.entries[i]
		a.wg.Add(1)
		go func() {
			defer a.wg.Done()
			for job := range entry.ch {
				a.deliverOne(entry, job.inc, job.action)
			}
		}()
	}
	go func() {
		<-ctx.Done()
		a.shutdown()
	}()
}

// shutdown waits for all delivery workers to finish (used in tests).
func (a *AlertManager) shutdown() {
	for i := range a.entries {
		close(a.entries[i].ch)
	}
	a.wg.Wait()
}

// DeadLetters returns a copy of the dead-letter ring buffer.
func (a *AlertManager) DeadLetters() interface{} {
	a.dlqMu.Lock()
	defer a.dlqMu.Unlock()
	n := 0
	for i := range a.dlqRing {
		if a.dlqRing[i].Timestamp.IsZero() {
			break
		}
		n++
	}
	out := make([]DeadLetterEntry, n)
	for i := 0; i < n; i++ {
		idx := (a.dlqHead - n + i + dlqCap) % dlqCap
		out[i] = a.dlqRing[idx]
	}
	return out
}

func (a *AlertManager) recordDeadLetter(entry *providerEntry, inc *model.Incident, action model.IncidentAction, err error) {
	a.dlqMu.Lock()
	defer a.dlqMu.Unlock()
	a.dlqRing[a.dlqHead] = DeadLetterEntry{
		Provider:  entry.provider.Name(),
		Key:       inc.Key,
		Action:    action,
		Error:     err.Error(),
		Timestamp: time.Now(),
	}
	a.dlqHead = (a.dlqHead + 1) % dlqCap
}

// deliverOne handles the full send+retry for a single (entry, incident) pair.
func (a *AlertManager) deliverOne(entry *providerEntry, inc *model.Incident, action model.IncidentAction) {
	p := entry.provider
	metrics.Default.NotificationsTotal.Add(1)

	maxLines := a.maxLogLines
	if maxLines <= 0 {
		maxLines = 100
	}
	tpl := entry.templates
	if len(tpl) == 0 {
		tpl = a.templates
	}
	msg := truncateMsg(formatIncidentMessage(inc, action, maxLines, tpl), entry.maxBytes)

	var err error
	if action == model.ActionDigestFlush {
		err = sendWithRetry(func() error {
			return p.SendMessage(msg)
		}, entry.maxAttempts, entry.retryDelay, entry.maxBackoff, p.Name())
	} else {
		if !shouldDeliver(entry.routes, inc) {
			klog.V(4).InfoS("incident filtered by route",
				"provider", p.Name(),
				"key", inc.Key)
			return
		}
		if tp, ok := p.(ThreadProvider); ok {
			err = sendWithRetry(func() error {
				return tp.SendIncident(inc, action)
			}, entry.maxAttempts, entry.retryDelay, entry.maxBackoff, p.Name())
		} else {
			err = sendWithRetry(func() error {
				return p.SendMessage(msg)
			}, entry.maxAttempts, entry.retryDelay, entry.maxBackoff, p.Name())
		}
	}
	if err != nil {
		metrics.Default.NotificationsDropped.Add(1)
		klog.ErrorS(err, "failed to send", "provider", p.Name(), "key", inc.Key, "id", inc.ID)
		a.recordDeadLetter(entry, inc, action, err)
		if entry.fallback != nil {
			fbMsg := msg
			fbErr := entry.fallback.provider.SendMessage("[fallback — primary " + p.Name() + " failed] " + fbMsg)
			if fbErr != nil {
				klog.ErrorS(fbErr, "fallback delivery failed", "provider", entry.fallback.provider.Name())
			}
		}
	}
}

// deliverAllSync sends directly to every provider (synchronous).
// Used before Start() is called (e.g. kwatch replay).
func (a *AlertManager) deliverAllSync(inc *model.Incident, action model.IncidentAction) {
	maxLines := a.maxLogLines
	if maxLines <= 0 {
		maxLines = 100
	}
	for _, entry := range a.entries {
		p := entry.provider
		tpl := entry.templates
		if len(tpl) == 0 {
			tpl = a.templates
		}
		msg := truncateMsg(formatIncidentMessage(inc, action, maxLines, tpl), entry.maxBytes)
		if action == model.ActionDigestFlush {
			if err := sendWithRetry(func() error {
				return p.SendMessage(msg)
			}, entry.maxAttempts, entry.retryDelay, entry.maxBackoff, p.Name()); err != nil {
				klog.ErrorS(err, "sync delivery failed", "provider", p.Name(), "key", inc.Key, "id", inc.ID)
			}
			continue
		}
		if !shouldDeliver(entry.routes, inc) {
			continue
		}
		var err error
		if tp, ok := p.(ThreadProvider); ok {
			err = sendWithRetry(func() error {
				return tp.SendIncident(inc, action)
			}, entry.maxAttempts, entry.retryDelay, entry.maxBackoff, p.Name())
		} else {
			err = sendWithRetry(func() error {
				return p.SendMessage(msg)
			}, entry.maxAttempts, entry.retryDelay, entry.maxBackoff, p.Name())
		}
		if err != nil {
			metrics.Default.NotificationsDropped.Add(1)
			klog.ErrorS(err, "sync delivery failed", "provider", p.Name(), "key", inc.Key, "id", inc.ID)
		}
	}
}

type templateData struct {
	Incident *model.Incident
	Action   string
	Message  string
}

func formatIncidentMessage(inc *model.Incident, action model.IncidentAction, maxLines int, templates map[string]*template.Template) string {
	var defaultMsg string
	switch action {
	case model.ActionCreate:
		defaultMsg = formatCreateMessage(inc, maxLines)
	case model.ActionUpdate:
		defaultMsg = formatUpdateMessage(inc, maxLines)
	case model.ActionResolved:
		defaultMsg = formatResolvedMessage(inc)
	case model.ActionDigestFlush:
		defaultMsg = inc.Hint
	default:
		return ""
	}
	if t, ok := templates[strings.ToLower(inc.Reason)]; ok {
		var buf bytes.Buffer
		err := t.Execute(&buf, templateData{
			Incident: inc,
			Action:   action.String(),
			Message:  defaultMsg,
		})
		if err == nil {
			return buf.String()
		}
		klog.ErrorS(err, "template render failed, falling back to default", "reason", inc.Reason)
	}
	return defaultMsg
}

func containerDisplayName(inc *model.Incident) string {
	if inc.ContainerName != "" {
		return inc.ContainerName
	}
	if len(inc.Containers) == 1 {
		for c := range inc.Containers {
			return c
		}
	}
	if len(inc.Containers) > 1 {
		names := make([]string, 0, len(inc.Containers))
		for c := range inc.Containers {
			names = append(names, c)
		}
		sort.Strings(names)
		return strings.Join(names, ", ")
	}
	return ""
}

func formatCreateMessage(inc *model.Incident, maxLines int) string {
	duration := inc.LastSeen.Sub(inc.FirstSeen).Round(time.Minute)

	severity := inc.Severity
	if severity == "" {
		severity = "normal"
	}

	containerName := containerDisplayName(inc)

	logsBlock := ""
	if inc.IncludeLogs && inc.Logs != "" {
		logsBlock = fmt.Sprintf("\nLogs:\n%s", truncateText(inc.Logs, maxLines))
	}
	eventsBlock := ""
	if inc.IncludeEvents && inc.Events != "" {
		eventsBlock = fmt.Sprintf("\nEvents:\n%s", truncateText(inc.Events, maxLines))
	}

	return fmt.Sprintf(
		"🚨 Incident: %s\nSeverity: %s\nOwner: %s (%s)\nNamespace: %s\nContainer: %s\nReason: %s\nRestarts: %d\nHint: %s%s%s\nPeak: %d resource(s)\nCount: %d\nDuration: %s",
		inc.Name, severity, inc.OwnerKind, inc.Name,
		inc.Namespace, containerName, inc.Reason,
		inc.RestartCount, inc.Hint,
		logsBlock, eventsBlock,
		inc.PeakResources, inc.Count, duration,
	)
}

func truncateMsg(s string, maxLen int) string {
	if maxLen <= 0 || len(s) <= maxLen {
		return s
	}
	suffix := "\n…(truncated)"
	cut := maxLen - len(suffix)
	if cut <= 0 {
		return suffix
	}
	// back up to a valid rune boundary (FIX-4)
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + suffix
}

func defaultMaxBytes(providerName string) int {
	switch strings.ToLower(providerName) {
	case "telegram":
		return 4096
	case "teams":
		return 28000
	case "slack":
		return 40000
	default:
		return 0 // unlimited
	}
}

func truncateText(s string, maxLines int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	return strings.Join(lines, "\n")
}

func formatUpdateMessage(inc *model.Incident, _ int) string {
	duration := inc.LastSeen.Sub(inc.FirstSeen).Round(time.Minute)

	severity := inc.Severity
	if severity == "" {
		severity = "normal"
	}

	containerName := containerDisplayName(inc)

	return fmt.Sprintf(
		"🔄 Update: %s | Severity: %s | Namespace: %s | Container: %s | Reason: %s | Count: %d | Duration: %s | Peak: %d resource(s)",
		inc.Name, severity, inc.Namespace, containerName, inc.Reason, inc.Count, duration, inc.PeakResources,
	)
}

func formatResolvedMessage(inc *model.Incident) string {
	duration := inc.LastSeen.Sub(inc.FirstSeen).Round(time.Minute)

	containerName := containerDisplayName(inc)

	return fmt.Sprintf(
		"✅ Resolved: %s | Namespace: %s | Container: %s | Reason: %s | Duration: %s | Total events: %d | Peak resources: %d",
		inc.Name, inc.Namespace, containerName, inc.Reason, duration, inc.Count, inc.PeakResources,
	)
}
