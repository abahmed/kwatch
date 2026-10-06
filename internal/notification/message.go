package notification

import "time"

// Message is a provider-neutral notification. Renderers turn it into plain
// text, Markdown, Slack blocks or HTML.
type Message struct {
	// Key identifies the conversation, so providers can edit or thread.
	Key string
	// DedupKey is the stable identity paging systems deduplicate on: it is
	// derived from the incident's root and failure mode, so it survives a
	// state reset and an old open alert is still found and resolved. Empty
	// means "use Key".
	DedupKey string `json:",omitempty"`
	// Revision orders messages of one conversation.
	Revision int
	Status   Status
	// Title is one line: what is wrong and where.
	//
	// Deprecated: renderers use Short (ShortText) instead. Title remains
	// the ShortText and Text fallback, the reason delivery's overflow
	// summary counts, and a field of the webhook, n8n and zapier payloads.
	Title string
	// Lines are the explanation sentences, in reading order.
	//
	// Deprecated: renderers use Note (NoteText) instead. Lines remain the
	// Text fallback and a field of the webhook, n8n and zapier payloads.
	Lines []string
	// Timeline lists the relevant events, oldest first.
	//
	// Deprecated: no renderer shows it. It remains a field of the
	// webhook, n8n and zapier payloads.
	Timeline []string
	// Output is the application's own recent output, redacted. Note never
	// contains it, so providers that show output render it beside Note.
	Output []string
	// Steps are commands or actions for the reader.
	Steps []Step
	// Confidence describes how sure the cause is, when there is one.
	Confidence string
	// Route is what provider routing rules match on.
	Route Route

	// Marker is the one status emoji of Note and Short: 🔴 page,
	// 🟠 notify, 🟡 low, ✅ resolved. Title and Lines never carry it.
	Marker string
	// Short is the lead sentence with its marker, for providers with
	// tight length limits (SMS-like pagers, push titles).
	Short string
	// Note is the whole message as a few plain sentences, the way an
	// engineer would write it: marker and lead, proof, consequence and
	// one suggested command. It has no labels, lists or links.
	Note string

	// Opens marks the message that announces its conversation, the first
	// one sent for Key. Delivery uses it to notice when a provider never
	// received the announcement. Until the composer sets it, delivery
	// treats an unresolved first revision as the opening.
	Opens bool `json:",omitempty"`
	// Opening is optional: on an update or resolve, the announcement of
	// the same conversation. Delivery sends it combined with this message
	// to a provider whose copy of the announcement was lost.
	Opening *Message `json:",omitempty"`
	// PagingOnly marks a message for the providers that track alerts by
	// key (paging tools and issue trackers) only: a startup announcement
	// the chat summary already covers, or the close of an incident whose
	// failures another incident took over. Chat channels read about both
	// elsewhere; an alert opened by key must still be closed by key.
	PagingOnly bool `json:",omitempty"`
	// SkipPaging marks the resolve of an incident whose announcement
	// never reached the paging and issue-tracker providers (it waited
	// for a digest, or a summary carried it). Those providers never
	// opened an alert for it, so a resolve would close nothing; chat
	// channels still get it.
	SkipPaging bool `json:",omitempty"`
	// Listed describes what a digest holds, for the audit log: counts and
	// the first incidents named. Nil for every other message.
	Listed *Listed `json:",omitempty"`
	// Carrier names the message that carries this one to people, when it
	// is not delivered on its own: "digest", "roll-up" or "startup
	// summary". Such a message exists for the audit log, which records
	// every decision when it is made; delivery drops it.
	Carrier string `json:",omitempty"`
	// ReopenWithin is set on the resolve of an incident that may reopen:
	// a failure within this long of the resolve continues the same
	// conversation. A threading provider keeps the thread that long, so
	// the "failing again" update replies in it.
	ReopenWithin time.Duration `json:",omitempty"`
	// Members is set on a roll-up: the conversation keys of the incidents
	// it announced. A provider that threads uses it to reply to the
	// roll-up when one of them updates or resolves.
	Members []string `json:",omitempty"`
}

// Listed is the content of a digest, summarised for the audit log.
type Listed struct {
	// Opened, Resolved and Risks count what the digest names.
	Opened, Resolved, Risks int
	// Items name the first incidents as "id: title", at most the bound
	// the producer chose; Opened+Resolved may be larger.
	Items []string `json:",omitempty"`
}

// IsOpening reports whether the message announces its conversation.
func (m Message) IsOpening() bool {
	if m.Resolved() {
		return false
	}
	return m.Opens || m.Revision <= 1
}

// IsPage reports whether the message is at the page tier, the one that
// wakes people up. Its route severity is "critical".
func (m Message) IsPage() bool {
	return NormalizeSeverity(m.Route.Severity) == SeverityCritical
}

// Route describes a message for provider routing: the namespaces it
// concerns, the finding reasons it contains, and its severity
// ("critical", "warning" or "info").
type Route struct {
	Namespaces []string
	Reasons    []string
	Severity   string
	// AnyOf is set on a message that names several problems (a startup
	// summary, roll-up, outage message or digest): the routes of those
	// problems. Such a message matches a provider's route when any one
	// of them does, so a provider routed by reason or namespace still
	// hears about a summary that contains something it would be sent.
	AnyOf []Route `json:",omitempty"`
}

// RouteSeverities are the only values Route.Severity takes, so they are
// the only values a provider route can usefully match.
var RouteSeverities = []string{"critical", "warning", "info"}

// IsRouteSeverity reports whether s, compared case-insensitively, is one of
// RouteSeverities.
func IsRouteSeverity(s string) bool {
	normalized := string(NormalizeSeverity(s))
	for _, severity := range RouteSeverities {
		if normalized == severity {
			return true
		}
	}
	return false
}

// Status drives the single status marker a renderer shows.
type Status uint8

// Statuses. Critical, Warning and Low follow the incident tiers page,
// notify and below; they share their markers with the composer.
const (
	StatusCritical Status = iota + 1
	StatusWarning
	StatusFlapping
	StatusResolved
	StatusLow
)

// Emoji is the one status marker for a status, the same marker the
// composer puts in Message.Marker. Flapping has no marker of its own:
// a flapping incident keeps its tier's marker, which Message.Marker
// carries. Without it, flapping falls back to the notify marker.
func (s Status) Emoji() string {
	switch s {
	case StatusCritical:
		return MarkerPage
	case StatusWarning, StatusFlapping:
		return MarkerNotify
	case StatusLow:
		return MarkerLow
	case StatusResolved:
		return MarkerResolved
	default:
		return ""
	}
}

// markerOf is the message's status marker: Marker, or the status's own
// when the message has none.
func (m Message) markerOf() string {
	if m.Marker != "" {
		return m.Marker
	}
	return m.Status.Emoji()
}

// Step is one suggested action. Mutating steps change the cluster and are
// labelled as such.
type Step struct {
	Text     string
	Command  string
	Mutating bool
}

// String names the status for machine consumers.
func (s Status) String() string {
	switch s {
	case StatusCritical:
		return "critical"
	case StatusWarning:
		return "warning"
	case StatusFlapping:
		return "flapping"
	case StatusResolved:
		return "resolved"
	default:
		// StatusLow, unset and unknown statuses are informational.
		return "info"
	}
}
