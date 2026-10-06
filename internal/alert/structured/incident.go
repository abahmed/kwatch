package structured

import "github.com/abahmed/kwatch/internal/notification"

// Incident is the structured incident schema. It is one shape for every
// message; a receiver groups updates by key and orders them by revision.
// Field meanings are documented in docs/providers.md.
type Incident struct {
	Cluster  string `json:"cluster"`
	Key      string `json:"key"`
	AlertKey string `json:"alertKey"`
	Revision int    `json:"revision"`
	Status   string `json:"status"`
	Resolved bool   `json:"resolved"`
	Marker   string `json:"marker"`
	Short    string `json:"short"`
	Note     string `json:"note"`
	// Markdown is the narrative as CommonMark: bold names, code spans,
	// bullets and a code block for the command. Note stays the plain
	// single paragraph.
	Markdown   string              `json:"markdown,omitempty"`
	Title      string              `json:"title"`
	Lines      []string            `json:"lines,omitempty"`
	Timeline   []string            `json:"timeline,omitempty"`
	Output     []string            `json:"output,omitempty"`
	Steps      []notification.Step `json:"steps,omitempty"`
	Confidence string              `json:"confidence,omitempty"`
	Route      notification.Route  `json:"route"`

	// Flags tell the receiver how delivery treated the message.
	Flags
}

// Flags are the delivery hints a receiver needs to keep its own state
// right. They are omitted when unset, so an ordinary message looks as it
// always did.
type Flags struct {
	// Opens marks the announcement of the incident.
	Opens bool `json:"opens,omitempty"`
	// PagingOnly marks a message meant for receivers that track alerts by
	// key: a startup announcement a summary already covers, or the close
	// of an incident whose failures another incident took over. A
	// receiver that opened the key must still close it.
	PagingOnly bool `json:"pagingOnly,omitempty"`
	// SkipPaging marks the resolve of an incident whose announcement was
	// never sent to receivers that track alerts by key (it waited for a
	// digest or a summary). Such a receiver has nothing to close.
	SkipPaging bool `json:"skipPaging,omitempty"`
	// Carrier names the message that carries this one to people instead
	// ("digest", "roll-up" or "startup summary").
	Carrier string `json:"carrier,omitempty"`
	// ReopenWithinSeconds is set on a resolve that may reopen: a failure
	// within this many seconds continues the same incident.
	ReopenWithinSeconds int64 `json:"reopenWithinSeconds,omitempty"`
}

// FlagsOf reads the delivery hints off a message.
func FlagsOf(m notification.Message) Flags {
	return Flags{
		Opens:               m.Opens,
		PagingOnly:          m.PagingOnly,
		SkipPaging:          m.SkipPaging,
		Carrier:             m.Carrier,
		ReopenWithinSeconds: int64(m.ReopenWithin.Seconds()),
	}
}

// NewIncident builds the payload for one message.
func NewIncident(cluster string, m notification.Message) Incident {
	return Incident{
		Cluster: cluster, Key: m.Key, AlertKey: m.AlertKey(cluster),
		Revision: m.Revision, Status: m.Status.String(),
		Resolved: m.Resolved(), Marker: m.Marker,
		Short: m.ShortText(), Note: m.NoteParagraph(),
		Markdown: markdown(m), Title: m.Title,
		Lines: m.Lines, Timeline: m.Timeline, Output: m.Output,
		Steps: m.Steps, Confidence: m.Confidence, Route: m.Route,
		Flags: FlagsOf(m),
	}
}

// markdown is the message's blocks as CommonMark.
func markdown(m notification.Message) string {
	if len(m.Doc) == 0 {
		return ""
	}
	return m.Render(notification.MarkdownDialect(nil, "\n\n"))
}
