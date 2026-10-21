package announce

import (
	"strconv"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
)

// MaxAuditItems bounds the incidents a digest's audit entry names with
// their titles. Past it the entry ends with one more item that counts the
// rest and lists their IDs (see hiddenItem), up to MaxHiddenIDs of them,
// so the audit log never hides which incidents a digest carried.
const MaxAuditItems = 100

// MaxHiddenIDs bounds the IDs the "and N more" item lists.
const MaxHiddenIDs = 200

// auditEntry is one incident of a digest as the audit log names it. The
// title is written only for the items that are shown.
type auditEntry struct {
	id    string
	title func() string
}

// listedInDigest summarises a digest for the audit log: the counts, the
// first MaxAuditItems incidents as "id: title", an "and N more" item
// that names the others by ID, and what the message rendered: its line
// count and its grouped, unchanged and wake lines.
func (c *Collector) listedInDigest(
	g *LowDigest, risks int, ongoing []ongoingItem, now time.Time,
	rendered *notification.Listed,
) *notification.Listed {
	listed := &notification.Listed{
		Opened: opened(g.Opened), Resolved: len(g.Resolved), Risks: risks,
		Ongoing: len(ongoing) + len(g.Opened) - opened(g.Opened),
	}
	if rendered != nil {
		listed.Lines, listed.Summaries = rendered.Lines, rendered.Summaries
	}
	listed.Items = auditItems(c.auditEntries(g, ongoing, now))
	return listed
}

// auditEntries lists every incident the digest names, in the order it
// names them: opened, resolved, then ongoing.
func (c *Collector) auditEntries(
	g *LowDigest, ongoing []ongoingItem, now time.Time,
) []auditEntry {
	var out []auditEntry
	add := func(decisions []incident.Decision, suffix string) {
		for _, d := range decisions {
			out = append(out, auditEntry{d.Incident.ID, func() string {
				return c.env.Messages.Write(d, now).Title + suffix
			}})
		}
	}
	add(g.Opened, "")
	add(g.Resolved, " (resolved)")
	for _, item := range ongoing {
		out = append(out, auditEntry{item.p.ID, func() string {
			return item.Line(now) + " (ongoing)"
		}})
	}
	return out
}

// auditItems words the entries: the first MaxAuditItems as "id: title",
// then one item for the rest.
func auditItems(entries []auditEntry) []string {
	var items []string
	for i, e := range entries {
		if i == MaxAuditItems {
			return append(items, hiddenItem(entries[i:]))
		}
		items = append(items, e.id+": "+e.title())
	}
	return items
}

// hiddenItem counts the entries the audit entry does not word and lists
// their IDs, so a reader can still find each of them.
func hiddenItem(rest []auditEntry) string {
	ids := make([]string, 0, min(len(rest), MaxHiddenIDs))
	for i, e := range rest {
		if i == MaxHiddenIDs {
			break
		}
		ids = append(ids, e.id)
	}
	text := "and " + strconv.Itoa(len(rest)) + " more: " +
		strings.Join(ids, ", ")
	if len(rest) > len(ids) {
		text += ", and " + strconv.Itoa(len(rest)-len(ids)) +
			" others"
	}
	return text
}
