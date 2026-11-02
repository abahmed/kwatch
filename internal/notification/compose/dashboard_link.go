package compose

import (
	"net/url"

	"github.com/abahmed/kwatch/internal/notification"
)

// Link lines. A message about one incident links its page; a message
// that lists several links the list of issues.
const (
	incidentLinkLead = "Open in dashboard: "
	listLinkLead     = "Open dashboard: "
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
	return withLink(msg, incidentLinkLead+base+"/issues/"+
		url.PathEscape(msg.Key))
}

// linkedList is msg with a link to the dashboard's list of issues.
func (w Writer) linkedList(msg notification.Message) notification.Message {
	base := w.dashboardBase()
	if base == "" {
		return msg
	}
	return withLink(msg, listLinkLead+base+"/issues")
}

// withLink adds line as the last block. It is plain text: every
// provider shows a bare address as a link. After a pinned fix the line
// is pinned too, so a cut message keeps the fix and its link together;
// otherwise a cut drops it first. A message without blocks is left
// alone, so the plain note never carries a link.
func withLink(msg notification.Message, line string) notification.Message {
	n := len(msg.Doc)
	if n == 0 {
		return msg
	}
	block := notification.Block{Kind: notification.Para,
		Spans:  []notification.Span{{Text: line}},
		Pinned: msg.Doc[n-1].Pinned}
	msg.Doc = append(append([]notification.Block(nil), msg.Doc...), block)
	return msg
}
