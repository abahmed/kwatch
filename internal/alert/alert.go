package alert

import (
	"fmt"
	"reflect"
	"strings"
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

type AlertManager struct {
	providers []Provider
}

// Provider interface
type Provider interface {
	Name() string
	SendEvent(*event.Event) error
	SendMessage(string) error
}

// Init initializes AlertManager with provided config
func (a *AlertManager) Init(
	alertCfg map[string]map[string]interface{},
	appCfg *config.App) {
	a.providers = make([]Provider, 0)
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
			a.providers = append(a.providers, pvdr)
		}
	}
}

// Notify sends string msg to all providers
func (a *AlertManager) Notify(msg string) {
	klog.InfoS("sending message", "msg", msg)

	for _, prv := range a.providers {
		if err := prv.SendMessage(msg); err != nil {
			klog.ErrorS(err,
				"failed to send msg",
				"provider", prv.Name())
		}
	}
}

// NotifyEvent sends event to all providers
func (a *AlertManager) NotifyEvent(event event.Event) {
	klog.InfoS("sending event", "event", event)

	for _, prv := range a.providers {
		if err := prv.SendEvent(&event); err != nil {
			klog.ErrorS(err,
				"failed to send event",
				"provider", prv.Name())
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
	msg := formatIncidentMessage(inc, action)
	klog.InfoS("sending incident", "action", action, "key", inc.Key, "count", inc.Count)
	for _, prv := range a.providers {
		if tp, ok := prv.(ThreadProvider); ok {
			if err := tp.SendIncident(inc, action); err != nil {
				klog.ErrorS(err,
					"failed to send incident via thread provider",
					"provider", prv.Name())
			}
		} else if err := prv.SendMessage(msg); err != nil {
			klog.ErrorS(err,
				"failed to send incident",
				"provider", prv.Name())
		}
	}
}

func formatIncidentMessage(inc *model.Incident, action model.IncidentAction) string {
	switch action {
	case model.ActionCreate:
		return formatCreateMessage(inc)
	case model.ActionUpdate:
		return formatUpdateMessage(inc)
	case model.ActionStale:
		return formatStaleMessage(inc)
	case model.ActionResolved:
		return formatResolvedMessage(inc)
	default:
		return ""
	}
}

func formatCreateMessage(inc *model.Incident) string {
	resources := len(inc.Resources)
	duration := inc.LastSeen.Sub(inc.FirstSeen).Round(time.Minute)

	logsBlock := ""
	if inc.Logs != "" {
		logsBlock = fmt.Sprintf("\nLogs:\n%s", truncateText(inc.Logs, 100))
	}
	eventsBlock := ""
	if inc.Events != "" {
		eventsBlock = fmt.Sprintf("\nEvents:\n%s", truncateText(inc.Events, 100))
	}

	return fmt.Sprintf(
		"🚨 Incident: %s\nOwner: %s (%s)\nNamespace: %s\nContainer: %s\nReason: %s\nRestarts: %d\nHint: %s%s%s\nAffected: %d resource(s)\nCount: %d\nDuration: %s",
		inc.Name, inc.OwnerKind, inc.Name,
		inc.Namespace, inc.ContainerName, inc.Reason,
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

func formatUpdateMessage(inc *model.Incident) string {
	resources := len(inc.Resources)
	duration := inc.LastSeen.Sub(inc.FirstSeen).Round(time.Minute)

	return fmt.Sprintf(
		"🔄 Update: %s | Namespace: %s | Reason: %s | Count: %d | Duration: %s | Affected: %d resource(s)",
		inc.Name, inc.Namespace, inc.Reason, inc.Count, duration, resources,
	)
}

func formatStaleMessage(inc *model.Incident) string {
	resources := len(inc.Resources)
	duration := inc.LastSeen.Sub(inc.FirstSeen).Round(time.Minute)

	return fmt.Sprintf(
		"⚠️ Stale: %s | Namespace: %s | Reason: %s | Last seen: %s | Count: %d | Duration: %s | Affected: %d resource(s)",
		inc.Name, inc.Namespace, inc.Reason,
		inc.LastSeen.Format("15:04:05"), inc.Count, duration, resources,
	)
}

func formatResolvedMessage(inc *model.Incident) string {
	duration := inc.LastSeen.Sub(inc.FirstSeen).Round(time.Minute)

	return fmt.Sprintf(
		"✅ Resolved: %s | Namespace: %s | Reason: %s | Duration: %s | Total events: %d",
		inc.Name, inc.Namespace, inc.Reason, duration, inc.Count,
	)
}
