package notification

import (
	"strings"
	"unicode/utf8"
)

// noticeKey groups plain operator messages (startup, upgrade, test) so
// paging systems deduplicate them into one alert instead of one per notice.
const noticeKey = "notice"

// NoteText is the full narrative a provider sends. It falls back to the
// structured text rendering when a message carries no Note.
func (m Message) NoteText() string {
	if note := strings.TrimSpace(m.Note); note != "" {
		return note
	}
	return Text(m)
}

// ShortText is the one-line lead a provider uses as a title or SMS body.
// It falls back to the marker and the title.
func (m Message) ShortText() string {
	if short := strings.TrimSpace(m.Short); short != "" {
		return short
	}
	return strings.TrimSpace(m.markerOf() + " " + m.Title)
}

// Resolved reports whether the message closes its incident.
func (m Message) Resolved() bool {
	return m.Status == StatusResolved
}

// AlertKey is the stable identity paging systems and issue trackers use to
// update and resolve one alert per incident. The cluster name is part of
// the key, so two clusters sending to one service account never merge or
// resolve each other's alerts. An empty cluster keeps the bare key.
func (m Message) AlertKey(cluster string) string {
	key := m.conversationKey()
	if m.DedupKey != "" {
		key = m.DedupKey
	}
	cluster = strings.Join(strings.Fields(cluster), "-")
	if cluster == "" {
		return "kwatch-" + key
	}
	return "kwatch-" + cluster + "-" + key
}

// ThreadKey is the per-provider identity of one incident conversation. It
// never contains the cluster name because each provider instance belongs to
// one cluster, and it is the key of persisted thread state, so its format
// must not change.
func (m Message) ThreadKey() string {
	return "kwatch-" + m.conversationKey()
}

// IsNotice reports whether the message is a plain operator notice (startup,
// upgrade, test) rather than an incident.
func (m Message) IsNotice() bool {
	return m.conversationKey() == noticeKey
}

// SummaryKeyPrefix starts the conversation key of every startup summary.
// compose.StartupKey builds the key; this prefix is how a provider tells a
// summary from an incident.
const SummaryKeyPrefix = "startup/"

// IsSummary reports whether the message is the startup summary or its
// closing resolve. The summary lists problems that already have their own
// conversations, so it is information, not an incident to page on.
func (m Message) IsSummary() bool {
	return strings.HasPrefix(m.Key, SummaryKeyPrefix)
}

// IsInformational reports whether the message only informs: a plain
// notice or the startup summary. Paging providers and issue trackers skip
// these, because nothing would ever close the alert or issue they open.
func (m Message) IsInformational() bool {
	return m.IsNotice() || m.IsSummary()
}

func (m Message) conversationKey() string {
	if m.Key == "" {
		return noticeKey
	}
	return m.Key
}

// Notice wraps a plain operator message for providers that can only send
// incidents. Every notice shares one alert key. A notice that starts
// with a status marker keeps it as its Marker and status; one without
// is a warning.
func Notice(text string) Message {
	text = strings.TrimSpace(text)
	short, _, _ := strings.Cut(text, "\n")
	msg := Message{
		Key: noticeKey, Status: StatusWarning,
		Title: short, Short: short, Note: text,
	}
	for _, marker := range Markers() {
		if rest, ok := strings.CutPrefix(short, marker+" "); ok {
			msg.Marker, msg.Status = marker, markerStatus(marker)
			msg.Title = rest
			break
		}
	}
	return msg
}

// markerStatus is the status a status marker stands for.
func markerStatus(marker string) Status {
	switch marker {
	case MarkerPage:
		return StatusCritical
	case MarkerLow:
		return StatusLow
	case MarkerResolved:
		return StatusResolved
	default:
		return StatusWarning
	}
}

// Truncate cuts text to at most limit bytes on a rune boundary, ending in
// an ellipsis. Cutting is deterministic, so a retried delivery sends the
// same bytes.
func Truncate(text string, limit int) string {
	if limit <= 0 || len(text) <= limit {
		return text
	}
	const ellipsis = "…"
	cut := limit - len(ellipsis)
	if cut <= 0 {
		return ""
	}
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut] + ellipsis
}

// MailSubject is the Short lead folded onto one line, so message text can
// never add a mail header.
func (m Message) MailSubject() string {
	return strings.Join(strings.Fields(m.ShortText()), " ")
}

// MailBody is the narrative followed by the application's recent output
// as a "> " quoted block, for plain-text mail.
func (m Message) MailBody() string {
	text := m.NoteText()
	if len(m.Output) == 0 {
		return text
	}
	quoted := make([]string, 0, len(m.Output))
	for _, line := range m.Output {
		quoted = append(quoted, "> "+line)
	}
	return text + "\n\n" + strings.Join(quoted, "\n")
}
