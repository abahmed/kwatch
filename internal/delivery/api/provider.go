package api

import "context"

// Provider is the canonical, cancellation-aware provider contract. Every
// provider renders incident messages itself and accepts plain operator
// messages (startup, upgrade, test) as text.
type Provider interface {
	Name() string
	IncidentProvider
	SendMessage(context.Context, string) error
}

// PlainMessageSkipper is implemented by providers that drop plain operator
// messages: paging systems and issue trackers, where a message would open
// an alert or issue that nothing closes. Delivery uses it to count the
// overflow summaries such a provider would silently discard.
type PlainMessageSkipper interface {
	SkipsPlainMessages() bool
}

// PagingOnlyReceiver is implemented by providers that take plain messages
// and also want paging-only messages: structured receivers (webhook, n8n,
// zapier, Splunk) that must see the close of an incident another incident
// took over. They are not paging tools, so they still get every notice
// and summary.
type PagingOnlyReceiver interface {
	ReceivesPagingOnly() bool
}
