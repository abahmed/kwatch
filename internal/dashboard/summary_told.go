package dashboard

import (
	"cmp"
	"slices"
	"strings"
)

// Channel kinds, as the summary and /api/kwatch group channels.
const (
	channelChat  = "chat"
	channelPager = "pager"
	channelEmail = "email"
	channelOther = "other"
)

// channelKinds sort the providers people read from by how they reach
// them. A provider not listed is "other"; one the send trail marks as
// holding alerts by key is a pager whatever this says.
var channelKinds = map[string]string{
	"slack": channelChat, "discord": channelChat, "teams": channelChat,
	"teamsworkflow": channelChat, "telegram": channelChat,
	"matrix": channelChat, "mattermost": channelChat,
	"rocketchat": channelChat, "googlechat": channelChat,
	"zulip": channelChat, "webex": channelChat, "flock": channelChat,
	"feishu": channelChat, "dingtalk": channelChat, "wecom": channelChat,
	"signal": channelChat, "threema": channelChat,
	"pagerduty": channelPager, "opsgenie": channelPager,
	"goalert": channelPager, "ilert": channelPager,
	"incidentio": channelPager, "incident.io": channelPager,
	"splunkoncall": channelPager, "squadcast": channelPager,
	"zenduty": channelPager, "signl4": channelPager,
	"email": channelEmail, "ses": channelEmail, "sendgrid": channelEmail,
	"mailgun": channelEmail, "resend": channelEmail,
}

// channelKind is how provider reaches people.
func channelKind(provider string, pager bool) string {
	if pager {
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
// send per provider from the trail, then each pager's own alert state,
// which is what the pager holds. Pagers lead, then by name.
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
		if t.Kind != channelPager {
			t.Kind = channelKind(a.Channel, a.Pager)
		}
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
		t.Kind, t.State = channelPager, p.State
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
