package compose

import (
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/notification"
)

// sourceLabel opens the line that links a change to where it came from.
const sourceLabel = "Source: "

// withSourceLinks adds, for a message that names a change, a line that
// links it to its source: the GitOps application that deployed it
// (when app.gitOpsLinks gives its web page) and the commit or
// repository recorded on the object. Only a line
// with at least one link is added; the note itself never carries one.
// A Community message names no change, so it gets none.
func (w Writer) withSourceLinks(
	d incident.Decision, msg notification.Message,
) notification.Message {
	if w.Community || len(msg.Doc) == 0 {
		return msg
	}
	change, ok := shownChange(d)
	if !ok {
		return msg
	}
	// The note names the image already; a message links only the
	// application and the commit.
	src := incident.SourceOf(change)
	src.Image = ""
	links := format.SourceLinks(src, w.GitOpsLinks)
	block, ok := sourceBlock(links)
	if !ok {
		return msg
	}
	block.Pinned = msg.Doc[len(msg.Doc)-1].Pinned
	msg.Doc = append(append([]notification.Block(nil), msg.Doc...), block)
	return msg
}

// shownChange is the change a message of d names: the change its cause
// blames, else, without a cause, the first recent change it lists.
func shownChange(d incident.Decision) (inventory.Change, bool) {
	if d.Action == incident.Resolve {
		return inventory.Change{}, false
	}
	if c := d.Incident.Cause; c != nil {
		if c.Change == nil {
			return inventory.Change{}, false
		}
		return *c.Change, true
	}
	for _, change := range d.Facts.Changes {
		if !systemChange(change) {
			return change, true
		}
	}
	return inventory.Change{}, false
}

// sourceBlock writes the links as one line: "Source: Argo CD app shop ·
// commit 1a2b3c4". Without any address it is not worth a line.
func sourceBlock(links []format.SourceLink) (notification.Block, bool) {
	spans := []notification.Span{{Text: sourceLabel}}
	linked := false
	for i, link := range links {
		if i > 0 {
			spans = append(spans, notification.Span{Text: " · "})
		}
		if link.URL == "" {
			spans = append(spans, notification.Span{Text: link.Label})
			continue
		}
		linked = true
		spans = append(spans, notification.LinkSpan(link.Label, link.URL))
	}
	return notification.Block{Kind: notification.Para, Spans: spans},
		linked
}
