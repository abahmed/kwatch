package dashboard

import (
	"strings"
	"unicode"
)

// channelNames are the product names a plain title-case would get wrong.
// Every other provider key is shown title-cased: slack is Slack.
var channelNames = map[string]string{
	"datadog":       "Datadog",
	"dingtalk":      "DingTalk",
	"github":        "GitHub",
	"gitlab":        "GitLab",
	"goalert":       "GoAlert",
	"googlechat":    "Google Chat",
	"homeassistant": "Home Assistant",
	"ifttt":         "IFTTT",
	"incident.io":   "incident.io",
	"incidentio":    "incident.io",
	"messagebird":   "MessageBird",
	"n8n":           "n8n",
	"newrelic":      "New Relic",
	"ntfy":          "ntfy",
	"opsgenie":      "Opsgenie",
	"pagerduty":     "PagerDuty",
	"rocketchat":    "Rocket.Chat",
	"sendgrid":      "SendGrid",
	"sensugo":       "Sensu Go",
	"ses":           "Amazon SES",
	"signl4":        "SIGNL4",
	"sns":           "Amazon SNS",
	"splunkoncall":  "Splunk On-Call",
	"teams":         "Microsoft Teams",
	"teamsworkflow": "Microsoft Teams",
	"wecom":         "WeCom",
	"clickup":       "ClickUp",
}

// channelName is how the dashboard names a provider key.
func channelName(key string) string {
	if name, ok := channelNames[strings.ToLower(key)]; ok {
		return name
	}
	r := []rune(key)
	if len(r) == 0 {
		return key
	}
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}
