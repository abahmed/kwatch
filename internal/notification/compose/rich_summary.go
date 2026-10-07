package compose

import (
	"strconv"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/notification"
)

// The messages that list several incidents (digest, roll-up, startup
// and restored summaries, namespace outage) read as a short list: a
// headline with the count, a "Problems" section with one bullet each,
// one line for what resolved, and the workloads that never become ready
// last. The
// Note keeps the old single paragraph for payloads that expect it.

// headline is the marker, a bold name and the facts after it:
// "🟡 kwatch digest · prod — 2 problems · 14 resolved".
func (w Writer) headline(
	mark, name string, facts []string,
) notification.Block {
	spans := []notification.Span{{Text: mark + " "},
		{Text: name, Style: notification.Bold}}
	if w.Cluster != "" {
		spans = append(spans, notification.Span{Text: " · " + w.Cluster})
	}
	if len(facts) > 0 {
		spans = append(spans, notification.Span{
			Text: " — " + strings.Join(facts, " · ")})
	}
	return notification.Block{Kind: notification.Para, Spans: spans}
}

func headingBlock(text string) notification.Block {
	return notification.Block{Kind: notification.Heading,
		Spans: []notification.Span{{Text: text}}}
}

// problemBullets is one bullet per incident, the first
// maxSummaryNamed, then "+N more".
func (w Writer) problemBullets(
	ds []incident.Decision, now time.Time,
) []notification.Block {
	ds = append([]incident.Decision(nil), ds...)
	sortByImpact(ds)
	var out []notification.Block
	for i, d := range ds {
		if i == maxSummaryNamed {
			out = append(out, notification.Block{Kind: notification.Bullet,
				Spans: []notification.Span{{Text: "+" +
					strconv.Itoa(len(ds)-i) + " more"}}})
			break
		}
		b := notification.Block{Kind: notification.Bullet,
			Spans: podSpans(itemLine(Writer{}.Write(d, now).Title,
				d.Incident.Root.Namespace) + usualTail(d))}
		blocks := []notification.Block{b}
		boldNames(blocks, incidentNames(d.Incident, ""))
		out = append(out, blocks[0])
	}
	return out
}

// itemLine shortens an incident's title for a list: the subject, a dash
// and what is wrong in a few words, with the namespace in brackets. See
// shortWhat.
func itemLine(title, namespace string) string {
	title = strings.TrimSuffix(strings.TrimSpace(title), ".")
	subject, rest, ok := strings.Cut(title, " is ")
	if !ok {
		subject, rest, ok = strings.Cut(title, " are ")
	}
	if !ok || strings.Contains(subject, `"`) {
		return title
	}
	if namespace != "" {
		subject = strings.Replace(subject, " in "+namespace,
			" ("+namespace+")", 1)
	}
	return subject + " — " + shortWhat(rest)
}

// resolvedBlock is the one line for what resolved.
func resolvedBlock(ds []incident.Decision, since string) notification.Block {
	var ids []inventory.EntityID
	for _, d := range ds {
		ids = append(ids, d.Incident.Root)
	}
	text := notification.MarkerResolved + " " +
		strconv.Itoa(len(ds)) + " resolved " + since + ": " +
		workloadList(ids)
	return notification.Block{Kind: notification.Para,
		Spans: []notification.Span{{Text: text}}}
}

// riskBlocks lists the workloads with pods that run but never become
// ready: a count and up to three workloads.
func riskBlocks(risks []detection.Finding) []notification.Block {
	groups := groupRisks(neverReady(risks))
	if len(groups) == 0 {
		return nil
	}
	out := []notification.Block{headingBlock(
		"Running but not ready")}
	for _, g := range groups {
		label := "Pods never ready"
		text := label + " — " + strconv.Itoa(len(g.ids)) + ": " +
			workloadList(g.ids)
		out = append(out, notification.Block{Kind: notification.Bullet,
			Spans: []notification.Span{{Text: text}}})
	}
	return out
}

// countFact is "2 problems" for a headline.
func countFact(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}

// listDoc is the structured form of a start-up, restored or roll-up
// message: headline, the problems, then the closing small print.
func (w Writer) listDoc(
	mark, name string, facts []string, ds []incident.Decision,
	now time.Time,
) []notification.Block {
	doc := []notification.Block{w.headline(mark, name, facts)}
	if len(ds) > 0 {
		doc = append(doc, headingBlock("Problems"))
		doc = append(doc, w.problemBullets(ds, now)...)
	}
	return append(doc, notification.Block{Kind: notification.Small,
		Spans: []notification.Span{{Text: eachOwnMessage}}})
}

// digestDoc is the structured digest.
func (w Writer) digestDoc(
	opened, resolved []incident.Decision, risks []detection.Finding,
	extras DigestExtras, now time.Time,
) []notification.Block {
	risks = neverReady(risks)
	var facts []string
	if len(opened) > 0 {
		facts = append(facts, countFact(len(opened), "problem"))
	}
	if n := len(extras.Ongoing); n > 0 {
		facts = append(facts, strconv.Itoa(n)+" ongoing")
	}
	if len(resolved) > 0 {
		facts = append(facts, strconv.Itoa(len(resolved))+" resolved")
	}
	if len(risks) > 0 {
		facts = append(facts, countFact(riskWorkloads(risks),
			"workload")+" not ready")
	}
	doc := []notification.Block{
		w.headline(notification.MarkerLow, "kwatch digest", facts)}
	if extras.Wake != nil {
		doc = append(doc, notification.Block{Kind: notification.Para,
			Spans: []notification.Span{{Text: extras.Wake.Text()}}})
	}
	if len(opened)+len(extras.Ongoing) > 0 {
		doc = append(doc, headingBlock("Problems"))
		doc = append(doc, w.problemBullets(opened, now)...)
		doc = append(doc, ongoingBullets(extras.Ongoing, now)...)
	}
	if len(resolved) > 0 {
		doc = append(doc, resolvedBlock(resolved, "since last digest"))
	}
	return append(doc, riskBlocks(risks)...)
}
