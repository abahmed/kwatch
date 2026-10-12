package delivery

import "strings"

type payloadPolicy struct {
	maxBytes int
	mode     string
}

const (
	payloadBounded     = "bounded"
	payloadProviderOwn = "provider_owned"
)

var boundedPayloadPolicies = map[string]payloadPolicy{
	"telegram":    {maxBytes: 4096, mode: payloadBounded},
	"teams":       {maxBytes: 28000, mode: payloadBounded},
	"slack":       {maxBytes: 40000, mode: payloadBounded},
	"discord":     {maxBytes: 2000, mode: payloadBounded},
	"pushover":    {maxBytes: 1024, mode: payloadBounded},
	"vonage":      {maxBytes: 1600, mode: payloadBounded},
	"plivo":       {maxBytes: 1600, mode: payloadBounded},
	"twilio":      {maxBytes: 1600, mode: payloadBounded},
	"messagebird": {maxBytes: 1600, mode: payloadBounded},
	"wecom":       {maxBytes: 4096, mode: payloadBounded},
	"feishu":      {maxBytes: 30000, mode: payloadBounded},
	// Providers whose service rejects or silently cuts a longer message.
	// The limits are the services' published ones, kept a little below.
	"ntfy":       {maxBytes: 4096, mode: payloadBounded},
	"webex":      {maxBytes: 7000, mode: payloadBounded},
	"zulip":      {maxBytes: 10000, mode: payloadBounded},
	"googlechat": {maxBytes: 4000, mode: payloadBounded},
	"matrix":     {maxBytes: 32000, mode: payloadBounded},
	"mattermost": {maxBytes: 16000, mode: payloadBounded},
	"dingtalk":   {maxBytes: 19000, mode: payloadBounded},
	"threema":    {maxBytes: 3400, mode: payloadBounded},
	// Workflows webhooks share the Teams card size limit.
	"teamsworkflow": {maxBytes: 28000, mode: payloadBounded},
	// Rocket.Chat rejects messages above 5000 characters by default.
	"rocketchat": {maxBytes: 4000, mode: payloadBounded},
}

// ownedPayloadProviders render a structured payload (JSON events, issue
// trackers, mail) or enforce their service limits in their own renderer, so
// cutting the text here would break the payload or duplicate that logic.
var ownedPayloadProviders = []string{
	"alerta", "clickup", "datadog", "email", "flock", "gitea", "github",
	"gitlab", "goalert", "gotify", "homeassistant", "ifttt", "ilert",
	"incident.io", "incidentio", "jira", "mailgun", "n8n", "newrelic",
	"opsgenie", "pagerduty", "pushbullet", "resend", "sendgrid", "sensugo",
	"ses", "signal", "signl4", "sns", "splunk", "splunkoncall",
	"squadcast", "webhook", "zapier", "zenduty",
}

// payloadPolicies holds one explicit entry per provider identity.
var payloadPolicies = buildPayloadPolicies()

func buildPayloadPolicies() map[string]payloadPolicy {
	policies := make(
		map[string]payloadPolicy,
		len(boundedPayloadPolicies)+len(ownedPayloadProviders),
	)
	for name, policy := range boundedPayloadPolicies {
		policies[name] = policy
	}
	for _, name := range ownedPayloadProviders {
		policies[name] = payloadPolicy{mode: payloadProviderOwn}
	}
	return policies
}

func providerPayloadPolicy(providerName string) payloadPolicy {
	policy, ok := payloadPolicies[strings.ToLower(providerName)]
	if ok {
		return policy
	}
	return payloadPolicy{mode: payloadProviderOwn}
}
