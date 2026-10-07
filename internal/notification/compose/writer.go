package compose

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/abahmed/kwatch/internal/notification"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// maxTimelineLines bounds the legacy timeline.
const maxTimelineLines = 6

// Writer composes messages. The zero value is ready to use.
type Writer struct {
	// Runbooks maps a lower-cased reason to a runbook URL.
	Runbooks map[string]string
	// Cluster is the configured cluster name. When set, every lead says
	// it once: "payments is down in shop (prod-eu-1) after …".
	Cluster string
}

// NewWriter builds a Writer for the cluster named cluster, which may be
// empty; runbook keys are matched case-insensitively.
func NewWriter(cluster string, runbooks map[string]string) Writer {
	lowered := make(map[string]string, len(runbooks))
	for reason, url := range runbooks {
		lowered[strings.ToLower(strings.TrimSpace(reason))] = url
	}
	return Writer{Runbooks: lowered, Cluster: strings.TrimSpace(cluster)}
}

// clusterTag is " (prod-eu-1)", or nothing without a cluster name.
func (w Writer) clusterTag() string {
	return caseFacts{cluster: w.Cluster}.clusterTag()
}

// Write composes the message for one decision.
func (w Writer) Write(d incident.Decision, now time.Time) notification.Message {
	return w.write(d, now, nil)
}

// WriteResolvedBy composes a resolve whose fixing change is known, so
// the note can say who fixed it and how.
func (w Writer) WriteResolvedBy(
	d incident.Decision, now time.Time, fix inventory.Change,
) notification.Message {
	return w.write(d, now, &fix)
}

func (w Writer) write(
	d incident.Decision, now time.Time, fix *inventory.Change,
) notification.Message {
	f := gatherFacts(d, now, fix)
	f.cluster = w.Cluster
	p := f.p
	msg := notification.Message{
		Key: p.ID, DedupKey: p.AlertKey, Revision: p.Revision,
		Route:    routeOfMessage(d.Action, p, f.members),
		Timeline: timeline(p, maxTimelineLines),
	}
	if d.Action == incident.Resolve {
		msg.Status = notification.StatusResolved
		mark, sentences := resolveNote(f)
		fill(&msg, mark, respell(arrange(sentences), d.Facts.KindNames))
		boldNames(msg.Doc, incidentNames(p, w.Cluster))
		msg.Opening = w.opening(d, now)
		if p.CanReopen() {
			msg.ReopenWithin = incident.RepageWindow
		}
		return msg
	}
	msg.Status = status(p)
	msg.Output = d.Facts.Output
	msg.Steps = append(nextSteps(p, stepMembers(p, f.members)),
		w.runbookSteps(f.members)...)
	msg.Confidence = confidence(p.Cause)
	fill(&msg, marker(p),
		respell(arrange(noteSentences(d, f)), d.Facts.KindNames))
	boldNames(msg.Doc, incidentNames(p, w.Cluster))
	msg.Opens = d.Action == incident.Announce
	if !msg.Opens {
		msg.Opening = w.opening(d, now)
	}
	return msg
}

// opening is the announcement of d's incident as it would be written
// now. Delivery sends it, with the update or resolve, to a provider
// that never received the incident's first message.
func (w Writer) opening(
	d incident.Decision, now time.Time,
) *notification.Message {
	announce := incident.Decision{Action: incident.Announce,
		Incident: d.Incident, Reason: d.Reason, Facts: d.Facts}
	if announce.Incident.State == incident.Resolved {
		// Written as it was while open: a resolved announcement would
		// read as a recovery.
		announce.Incident.State = incident.Open
	}
	msg := w.write(announce, now, nil)
	return &msg
}

// noteSentences are the sentences of an announcement or an update.
func noteSentences(d incident.Decision, f caseFacts) []sentence {
	if d.Action == incident.Update {
		return append(updateSentences(f), freshEvidenceSentences(f)...)
	}
	return append(append(leadSentences(f), writeAll(f, noteWriters)...),
		ackAnnouncedSentences(f)...)
}

// status is the machine-readable status of an incident's message. It
// follows the tier, like the marker, except that flapping is named as
// such; a flapping message still shows its tier's marker.
func status(p incident.Incident) notification.Status {
	if p.State == incident.Flapping {
		return notification.StatusFlapping
	}
	return tierStatus(p.Tier)
}

// tierStatus is the status of a tier, matching its marker.
func tierStatus(tier incident.Tier) notification.Status {
	switch tier {
	case incident.Page:
		return notification.StatusCritical
	case incident.Notify:
		return notification.StatusWarning
	default:
		return notification.StatusLow
	}
}

// rootFinding is the root's own most severe condition, if it has one.
// Symptoms (a workload below its replicas) never lead the message.
func rootFinding(
	p incident.Incident, members []detection.Finding,
) *detection.Finding {
	for i := range members {
		if members[i].Entity == p.Root && !members[i].Symptom &&
			!members[i].Advisory {
			return &members[i]
		}
	}
	return nil
}

func nonEmpty(value string) []string {
	if value == "" {
		return nil
	}
	return []string{value}
}

func confidence(cause *rootcause.CauseRecord) string {
	return certaintyOf(cause).word()
}

func sortedMembers(p incident.Incident) []detection.Finding {
	out := make([]detection.Finding, 0, len(p.Members))
	for _, s := range p.Members {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Severity != out[j].Severity {
			return out[i].Severity > out[j].Severity
		}
		if !out[i].Since.Equal(out[j].Since) {
			return out[i].Since.Before(out[j].Since)
		}
		// Members live in a map: break ties so a note never depends on
		// iteration order.
		if out[i].Entity != out[j].Entity {
			return out[i].Entity.String() < out[j].Entity.String()
		}
		return out[i].Reason < out[j].Reason
	})
	return out
}

func describe(id inventory.EntityID) string {
	name := id.Name
	if id.Namespace != "" {
		name = id.Namespace + "/" + id.Name
	}
	return string(id.Kind) + " " + name
}

func limit(values []string, n int) []string {
	if len(values) > n {
		return values[:n]
	}
	return values
}

func more(total, shown int) string {
	if total <= shown {
		return ""
	}
	return fmt.Sprintf(" and %d more", total-shown)
}

func upperFirst(value string) string {
	// Ranging over a string steps by rune, so the first letter is cut
	// whole even when it takes several bytes ("élan", "über").
	for i := range value {
		if i > 0 {
			return strings.ToUpper(value[:i]) + value[i:]
		}
	}
	return strings.ToUpper(value)
}

// lowerFirst lower-cases the first letter, except in an acronym such
// as "TLS" or "DNS", which keeps its capitals.
func lowerFirst(value string) string {
	if value == "" || isAcronym(value) ||
		strings.HasPrefix(value, "Kubernetes") {
		return value
	}
	// Cut the first rune whole, as upperFirst does.
	for i := range value {
		if i > 0 {
			return strings.ToLower(value[:i]) + value[i:]
		}
	}
	return strings.ToLower(value)
}

// routeOfMessage is the route of a message. The announcement is routed
// from the live members. Every later message is routed with what the
// announcement named (Incident.AnnouncedRoute, kept across restarts), or
// a resolve with no members left would match no rule that asked for a
// reason: a resolve takes the announced route whole, an update adds the
// live members to it and keeps its live severity.
func routeOfMessage(
	action incident.Action, p incident.Incident,
	members []detection.Finding,
) notification.Route {
	live := route(p, members)
	told := p.AnnouncedRoute
	if action == incident.Announce || told == nil {
		return live
	}
	if action == incident.Resolve {
		return notification.Route{
			Namespaces: append([]string(nil), told.Namespaces...),
			Reasons:    append([]string(nil), told.Reasons...),
			Severity:   told.Severity,
			Owners:     append([]string(nil), told.Owners...),
		}
	}
	return notification.Route{
		Namespaces: mergeSorted(told.Namespaces, live.Namespaces),
		Reasons:    mergeSorted(told.Reasons, live.Reasons),
		Severity:   live.Severity,
		Owners:     mergeSorted(told.Owners, live.Owners),
	}
}

// mergeSorted is the sorted union of two string lists.
func mergeSorted(a, b []string) []string {
	set := map[string]bool{}
	for _, v := range append(append([]string(nil), a...), b...) {
		set[v] = true
	}
	return sortedSet(set)
}

// route summarises an incident for provider routing rules.
func route(
	p incident.Incident, members []detection.Finding,
) notification.Route {
	namespaces := map[string]bool{}
	reasons := map[string]bool{}
	if p.Root.Namespace != "" {
		namespaces[p.Root.Namespace] = true
	}
	for _, s := range members {
		if s.Entity.Namespace != "" {
			namespaces[s.Entity.Namespace] = true
		}
		reasons[s.Reason] = true
	}
	severity := "warning"
	switch p.Tier {
	case incident.Page:
		severity = "critical"
	case incident.Digest, incident.Silent:
		severity = "info"
	}
	return notification.Route{
		Namespaces: sortedSet(namespaces), Reasons: sortedSet(reasons),
		Severity: severity, Owners: ownerList(p.Owner),
	}
}

// ownerList is the owner as the list a route carries.
func ownerList(owner string) []string {
	if owner == "" {
		return nil
	}
	return []string{owner}
}

func sortedSet(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

// isAcronym reports whether text starts with two capital letters.
func isAcronym(text string) bool {
	first, size := utf8.DecodeRuneInString(text)
	second, _ := utf8.DecodeRuneInString(text[size:])
	return size > 0 && unicode.IsUpper(first) && unicode.IsUpper(second)
}
