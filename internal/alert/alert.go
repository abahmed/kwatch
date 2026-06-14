package alert

import (
	"bytes"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"text/template"
	"time"

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
	"github.com/abahmed/kwatch/internal/model"
	"k8s.io/klog/v2"
)

type providerEntry struct {
	provider      Provider
	routes        []config.AlertRoute
	maxAttempts   int
	retryDelay    time.Duration
	fallback      *providerEntry
	fallbackNamed string                   // resolved in second pass
	templates     map[string]*template.Template
}

type AlertManager struct {
	entries      []providerEntry
	silences     []silenceMatcher
	maxLogLines  int
	templates    map[string]*template.Template
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
	namespaces []string
	reasons    []string
	podPattern []*regexp.Regexp
}

// Provider interface
type Provider interface {
	Name() string
	SendEvent(*event.Event) error
	SendMessage(string) error
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

func extractRetry(cfg map[string]interface{}) (maxAttempts int, delay time.Duration) {
	maxAttempts = 1
	delay = time.Second
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
		}
	}
	return
}

// Init initializes AlertManager with provided config
func (a *AlertManager) Init(
	alertCfg map[string]map[string]interface{},
	appCfg *config.App,
) {
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
			klog.InfoS("unknown alert provider, skipping", "name", k)
			continue
		}
		if !reflect.ValueOf(pvdr).IsNil() {
			maxAttempts, retryDelay := extractRetry(v)
			fbName := ""
			if raw, ok := v["fallback"]; ok {
				fbName, _ = raw.(string)
			}
			entries = append(entries, providerEntry{
				provider:      pvdr,
				routes:        extractRoutes(v),
				maxAttempts:   maxAttempts,
				retryDelay:    retryDelay,
				fallback:      nil,
				fallbackNamed: fbName,
				templates:     extractTemplates(v),
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
			namespaces: sr.Namespaces,
			reasons:    sr.Reasons,
		}
		for _, p := range sr.PodNamePatterns {
			if re, err := regexp.Compile(p); err == nil {
				sm.podPattern = append(sm.podPattern, re)
			} else {
				klog.ErrorS(err, "invalid silence pod name pattern", "pattern", p)
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

func sendWithRetry(sendFn func() error, maxAttempts int, delay time.Duration, providerName string) error {
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := sendFn(); err != nil {
			lastErr = err
			if attempt < maxAttempts {
				klog.V(4).InfoS("retrying provider delivery",
					"provider", providerName,
					"attempt", attempt,
					"maxAttempts", maxAttempts)
				time.Sleep(delay)
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

// Notify sends string msg to all providers
func (a *AlertManager) Notify(msg string) {
	klog.InfoS("sending message", "msg", msg)

	for _, entry := range a.entries {
		p := entry.provider
		if err := sendWithRetry(func() error {
			return p.SendMessage(msg)
		}, entry.maxAttempts, entry.retryDelay, p.Name()); err != nil && entry.fallback != nil {
			entry.fallback.provider.SendMessage("[fallback — primary " + p.Name() + " failed] " + msg)
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
		}, entry.maxAttempts, entry.retryDelay, p.Name()); err != nil && entry.fallback != nil {
			entry.fallback.provider.SendMessage("[fallback — primary " + p.Name() + " failed] " + event.Reason + " in " + event.Namespace + "/" + event.PodName)
		}
	}
}

// ThreadProvider is an optional interface for providers that support
// incident-aware messaging (e.g., Slack threads).
type ThreadProvider interface {
	SendIncident(inc *model.Incident, action model.IncidentAction) error
}

// NotifyIncident sends incident summary to all providers.
// Providers implementing ThreadProvider receive structured incident data;
// others fall back to plain text.
func (a *AlertManager) NotifyIncident(inc *model.Incident, action model.IncidentAction) {
	if action == model.ActionSkip {
		return
	}

	if a.isSilenced(inc) {
		klog.V(4).InfoS("incident suppressed by silence rule",
			"key", inc.Key, "reason", inc.Reason, "namespace", inc.Namespace)
		return
	}

	maxLines := a.maxLogLines
	if maxLines <= 0 {
		maxLines = 100
	}
	klog.InfoS("sending incident", "action", action, "key", inc.Key, "count", inc.Count)
	for _, entry := range a.entries {
		p := entry.provider
		var err error
		// per-provider templates fall back to global
		tpl := entry.templates
		if len(tpl) == 0 {
			tpl = a.templates
		}
		msg := formatIncidentMessage(inc, action, maxLines, tpl)
		if action == model.ActionDigestFlush {
			// digests bypass route filtering (no namespace) and ThreadProvider
			err = sendWithRetry(func() error {
				return p.SendMessage(msg)
			}, entry.maxAttempts, entry.retryDelay, p.Name())
		} else {
			if !shouldDeliver(entry.routes, inc) {
				klog.V(4).InfoS("incident filtered by route",
					"provider", p.Name(),
					"key", inc.Key)
				continue
			}
			if tp, ok := p.(ThreadProvider); ok {
				err = sendWithRetry(func() error {
					return tp.SendIncident(inc, action)
				}, entry.maxAttempts, entry.retryDelay, p.Name())
			} else {
				err = sendWithRetry(func() error {
					return p.SendMessage(msg)
				}, entry.maxAttempts, entry.retryDelay, p.Name())
			}
		}
		if err != nil {
			klog.ErrorS(err, "failed to send", "provider", p.Name(), "key", inc.Key)
			if entry.fallback != nil {
				fbMsg := msg
				fbErr := entry.fallback.provider.SendMessage("[fallback — primary " + p.Name() + " failed] " + fbMsg)
				if fbErr != nil {
					klog.ErrorS(fbErr, "fallback delivery failed", "provider", entry.fallback.provider.Name())
				}
			}
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

func formatCreateMessage(inc *model.Incident, maxLines int) string {
	resources := len(inc.Resources)
	duration := inc.LastSeen.Sub(inc.FirstSeen).Round(time.Minute)

	severity := inc.Severity
	if severity == "" {
		severity = "normal"
	}

	containerName := inc.ContainerName
	if len(inc.Containers) > 1 {
		names := make([]string, 0, len(inc.Containers))
		for c := range inc.Containers {
			names = append(names, c)
		}
		sort.Strings(names)
		containerName = strings.Join(names, ", ")
	}

	logsBlock := ""
	if inc.IncludeLogs && inc.Logs != "" {
		logsBlock = fmt.Sprintf("\nLogs:\n%s", truncateText(inc.Logs, maxLines))
	}
	eventsBlock := ""
	if inc.IncludeEvents && inc.Events != "" {
		eventsBlock = fmt.Sprintf("\nEvents:\n%s", truncateText(inc.Events, maxLines))
	}

	return fmt.Sprintf(
		"🚨 Incident: %s\nSeverity: %s\nOwner: %s (%s)\nNamespace: %s\nContainer: %s\nReason: %s\nRestarts: %d\nHint: %s%s%s\nAffected: %d resource(s)\nCount: %d\nDuration: %s",
		inc.Name, severity, inc.OwnerKind, inc.Name,
		inc.Namespace, containerName, inc.Reason,
		inc.RestartCount, inc.Hint,
		logsBlock, eventsBlock,
		resources, inc.Count, duration,
	)
}

func truncateText(s string, maxLines int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	return strings.Join(lines, "\n")
}

func formatUpdateMessage(inc *model.Incident, _ int) string {
	resources := len(inc.Resources)
	duration := inc.LastSeen.Sub(inc.FirstSeen).Round(time.Minute)

	severity := inc.Severity
	if severity == "" {
		severity = "normal"
	}

	return fmt.Sprintf(
		"🔄 Update: %s | Severity: %s | Namespace: %s | Reason: %s | Count: %d | Duration: %s | Affected: %d resource(s)",
		inc.Name, severity, inc.Namespace, inc.Reason, inc.Count, duration, resources,
	)
}

func formatResolvedMessage(inc *model.Incident) string {
	duration := inc.LastSeen.Sub(inc.FirstSeen).Round(time.Minute)

	return fmt.Sprintf(
		"✅ Resolved: %s | Namespace: %s | Reason: %s | Duration: %s | Total events: %d",
		inc.Name, inc.Namespace, inc.Reason, duration, inc.Count,
	)
}
