package dashboard

import (
	"cmp"
	"slices"
	"strings"

	providercatalog "github.com/abahmed/kwatch/internal/provider/catalog"
)

// Channel kinds, as the summary and /api/kwatch group channels.
const (
	channelChat    = "chat"
	channelPager   = "pager"
	channelTicket  = "ticket"
	channelWebhook = "webhook"
	channelEmail   = "email"
	channelOther   = "other"
)

// channelKinds sort the providers people read from by how they reach
// them. A provider not listed is "other". Pagers are not listed: a
// provider is a pager only when it pages a person
// (providercatalog.PagesPerson), never because it keeps an alert by key
// as issue trackers and webhooks do.
var channelKinds = map[string]string{
	"slack": channelChat, "discord": channelChat, "teams": channelChat,
	"teamsworkflow": channelChat, "telegram": channelChat,
	"matrix": channelChat, "mattermost": channelChat,
	"rocketchat": channelChat, "googlechat": channelChat,
	"zulip": channelChat, "webex": channelChat, "flock": channelChat,
	"feishu": channelChat, "dingtalk": channelChat, "wecom": channelChat,
	"signal": channelChat, "threema": channelChat,
	"github": channelTicket, "gitlab": channelTicket,
	"gitea": channelTicket, "jira": channelTicket,
	"clickup": channelTicket, "servicenow": channelTicket,
	"webhook": channelWebhook, "n8n": channelWebhook,
	"zapier": channelWebhook, "sns": channelWebhook,
	"email": channelEmail, "ses": channelEmail, "sendgrid": channelEmail,
	"mailgun": channelEmail, "resend": channelEmail,
}

// channelKind is how provider reaches people.
func channelKind(provider string) string {
	if providercatalog.PagesPerson(provider) {
		return channelPager
	}
	if k, ok := channelKinds[strings.ToLower(provider)]; ok {
		return k
	}
	return channelOther
}

// toldStates map a send's result to what the channel holds; results
// that reached no provider are left out.
var toldStates = map[string]string{
	deliveredOK: "sent",
	"failed":    "failed",
	"retrying":  "pending",
}

// toldOf is each channel's latest state for the incident: the latest
// send per provider from the trail, then the alert state of each
// provider that keeps one alert by key (a pager, an issue tracker or a
// webhook), which is what that provider holds. Pagers lead, then by
// name.
func toldOf(d *IncidentDetail) []SummaryTold {
	byProvider := map[string]*SummaryTold{}
	for _, a := range d.Deliveries {
		state, ok := toldStates[a.Result]
		if a.Channel == "" || !ok {
			continue
		}
		t := byProvider[a.Channel]
		if t == nil {
			t = &SummaryTold{Provider: a.Channel,
				Name: channelName(a.Channel)}
			byProvider[a.Channel] = t
		}
		t.Kind = channelKind(a.Channel)
		if !a.At.Before(t.At) {
			t.State, t.At = state, a.At
		}
	}
	for _, p := range d.Pager {
		t := byProvider[p.Provider]
		if t == nil {
			t = &SummaryTold{Provider: p.Provider,
				Name: channelName(p.Provider)}
			byProvider[p.Provider] = t
		}
		t.Kind, t.State = channelKind(p.Provider), p.State
		t.At = cmp.Or(p.ClosedAt, p.PagedAt, t.At)
	}
	out := make([]SummaryTold, 0, len(byProvider))
	for _, t := range byProvider {
		out = append(out, *t)
	}
	slices.SortFunc(out, func(a, b SummaryTold) int {
		return cmp.Or(cmp.Compare(pagerFirst(a.Kind), pagerFirst(b.Kind)),
			cmp.Compare(a.Name, b.Name), cmp.Compare(a.Provider,
				b.Provider))
	})
	return out
}

func pagerFirst(kind string) int {
	if kind == channelPager {
		return 0
	}
	return 1
}
