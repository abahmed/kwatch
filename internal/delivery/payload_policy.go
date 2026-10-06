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

var payloadPolicies = map[string]payloadPolicy{
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
}

func providerPayloadPolicy(providerName string) payloadPolicy {
	policy, ok := payloadPolicies[strings.ToLower(providerName)]
	if ok {
		return policy
	}
	return payloadPolicy{mode: payloadProviderOwn}
}
