package compose

import (
	"net/url"

	"github.com/abahmed/kwatch/internal/notification"
)

// Link labels. A message about one incident links its page; a message
// that lists several links the list of issues.
const (
	incidentLinkLabel = "Open in dashboard"
	listLinkLabel     = "Open dashboard"
)

// dashboardBase is the dashboard's public address, or "" when it has
// none (the dashboard is off, or nothing exposes it).
func (w Writer) dashboardBase() string {
	if w.Dashboard == nil {
		return ""
	}
	return w.Dashboard()
}

// linked is msg with a link to its incident's dashboard page.
func (w Writer) linked(msg notification.Message) notification.Message {
	base := w.dashboardBase()
	if base == "" || msg.Key == "" {
		return msg
	}
	return withLink(msg, incidentLinkLabel,
		base+"/issues/"+url.PathEscape(msg.Key))
}

// linkedList is msg with a link to the dashboard's list of issues.
func (w Writer) linkedList(msg notification.Message) notification.Message {
	base := w.dashboardBase()
	if base == "" {
		return msg
	}
	return withLink(msg, listLinkLabel, base+"/issues")
}

// withLink adds a line that shows label and links to address as the
// last block. A provider with links shows the label as one; plain text
// reads "label: address". After a pinned fix the line is pinned too, so
// a cut message keeps the fix and its link together; otherwise a cut
// drops it first. A message without blocks is left alone, so the plain
// note never carries a link.
func withLink(
	msg notification.Message, label, address string,
) notification.Message {
	n := len(msg.Doc)
	if n == 0 {
		return msg
	}
	block := notification.Block{Kind: notification.Para,
		Spans: []notification.Span{
			notification.LinkSpan(label, address)},
		Pinned: msg.Doc[n-1].Pinned}
	msg.Doc = append(append([]notification.Block(nil), msg.Doc...), block)
	return msg
}
