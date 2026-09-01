package config

// KnownProviders is the canonical set of known alert provider names.
// Both alert.Init and config validation reference this to prevent drift.
var KnownProviders = map[string]bool{
	"slack": true, "pagerduty": true, "discord": true, "telegram": true,
	"teams": true, "email": true, "rocketchat": true, "mattermost": true,
	"opsgenie": true, "matrix": true, "dingtalk": true, "feishu": true,
	"webhook": true, "zenduty": true, "googlechat": true,
	"gotify": true, "ntfy": true, "pushover": true, "webex": true,
	"github": true, "line": true,
	"gitlab": true, "gitea": true, "zapier": true, "n8n": true, "ifttt": true,
	"teamsworkflow": true, "zulip": true, "homeassistant": true,
	"splunk": true, "datadog": true,
	"newrelic": true, "clickup": true, "ilert": true,
	"incidentio": true, "incident.io": true, "squadcast": true, "signl4": true,
	"twilio": true, "vonage": true, "plivo": true,
	"messagebird": true, "signal": true, "sendgrid": true, "ses": true,
	"sns": true, "jira": true, "wecom": true, "splunkoncall": true,
	"mailgun": true, "resend": true, "goalert": true, "alerta": true,
	"threema": true, "flock": true, "pushbullet": true, "sensugo": true,
}
