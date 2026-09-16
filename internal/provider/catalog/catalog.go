// Package catalog contains provider identities shared by configuration and
// the statically linked provider factory catalog. It has no provider or
// configuration dependencies, so it can remain a leaf package.
package catalog

import "sort"

var providerNames = []string{
	"alerta", "clickup", "datadog", "dingtalk", "discord", "email",
	"feishu", "flock", "gitea", "github", "gitlab", "goalert",
	"googlechat", "gotify", "homeassistant", "ifttt", "ilert",
	"incident.io", "incidentio", "jira", "line", "mailgun", "matrix",
	"mattermost", "messagebird", "n8n", "newrelic", "ntfy", "opsgenie",
	"pagerduty", "plivo", "pushbullet", "pushover", "resend",
	"rocketchat", "sendgrid", "sensugo", "ses", "signal", "signl4",
	"slack", "sns", "splunk", "splunkoncall", "squadcast", "teams",
	"teamsworkflow", "telegram", "threema", "twilio", "vonage", "webex",
	"webhook", "wecom", "zapier", "zenduty", "zulip",
}

var providerAliases = map[string]string{
	"incident.io": "incidentio",
}

// Names returns all accepted provider keys in deterministic order.
func Names() []string {
	result := append([]string(nil), providerNames...)
	sort.Strings(result)
	return result
}

// Aliases returns the intentional alternate provider keys. The returned map
// is detached so callers cannot change catalog identity at runtime.
func Aliases() map[string]string {
	result := make(map[string]string, len(providerAliases))
	for alias, canonical := range providerAliases {
		result[alias] = canonical
	}
	return result
}
