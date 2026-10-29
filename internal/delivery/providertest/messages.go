package providertest

import (
	"strings"

	"github.com/abahmed/kwatch/internal/notification"
)

// Key is the incident key of every fixture message.
const Key = "p-42"

// Announce is the first message of an incident.
func Announce() notification.Message {
	return notification.Message{
		Key: Key, Revision: 1, Status: notification.StatusCritical,
		Title:  "payments crashed in shop",
		Lines:  []string{"payments ran out of memory 3 times."},
		Marker: notification.MarkerPage,
		Short:  "🔴 payments in shop is crash-looping.",
		Note: "🔴 payments in shop is crash-looping. It ran out of " +
			"memory 3 times in 10 minutes. Try " +
			"kubectl -n shop describe pod payments-1.",
		Output: []string{"panic: out of memory"},
		Route: notification.Route{
			Namespaces: []string{"shop"}, Reasons: []string{"OOMKilled"},
			Severity: "critical",
		},
	}
}

// Update is a later revision of the same incident.
func Update() notification.Message {
	m := Announce()
	m.Revision = 2
	m.Short = "🔴 payments in shop is still crash-looping."
	m.Note = "🔴 payments in shop is still crash-looping. It restarted " +
		"5 times now."
	return m
}

// Resolve closes the incident.
func Resolve() notification.Message {
	m := Announce()
	m.Revision = 3
	m.Status = notification.StatusResolved
	m.Marker = notification.MarkerResolved
	m.Output = nil
	m.Short = "✅ payments in shop is healthy again."
	m.Note = "✅ payments in shop is healthy again. It ran for 10 " +
		"minutes without a restart."
	return m
}

// Hostile carries markup and a broadcast mention, for escaping tests.
func Hostile() notification.Message {
	m := Announce()
	m.Short = "🔴 <b>pay</b> & <script>alert(1)</script>"
	m.Note = "🔴 <b>pay</b> & <script>alert(1)</script> @channel"
	return m
}

// Case is one step of the incident lifecycle.
type Case struct {
	Name    string
	Message notification.Message
}

// Lifecycle returns announce, update and resolve, in order.
func Lifecycle() []Case {
	return []Case{
		{Name: "announce", Message: Announce()},
		{Name: "update", Message: Update()},
		{Name: "resolve", Message: Resolve()},
	}
}

// Summary is a startup summary: one message listing problems that already
// existed at startup, each of which has its own conversation.
func Summary() notification.Message {
	return notification.Message{
		Key:      notification.SummaryKeyPrefix + "20260102T030405.000Z",
		Revision: 1, Status: notification.StatusWarning,
		Title: "kwatch started and found 2 problems",
		Short: "🟠 kwatch started and found 2 problems.",
		Note:  "🟠 kwatch started and found 2 problems that were there.",
	}
}

// Rich is an incident with structured blocks: a headline with bold
// names, a quoted pod error, a line with the affected service and the
// suggested command in a code block. The pod text is hostile on purpose:
// it holds markup and a broadcast mention.
func Rich() notification.Message {
	m := Announce()
	m.Note = "🔴 payments in shop is crash-looping. It said " +
		"\"boom *x* @channel\". Service web can't serve traffic. " +
		"Run kubectl logs payments -n shop"
	m.Doc = []notification.Block{
		{Kind: notification.Para, Spans: []notification.Span{
			{Text: "🔴 "},
			{Text: "payments", Style: notification.Bold},
			{Text: " in "},
			{Text: "shop", Style: notification.Bold},
			{Text: " is crash-looping."}}},
		{Kind: notification.Para, Spans: []notification.Span{
			{Text: "It said "},
			{Text: "boom *x* @channel", Style: notification.Code},
			{Text: "."}}},
		{Kind: notification.Para, Spans: []notification.Span{
			{Text: "Service "},
			{Text: "web", Style: notification.Bold},
			{Text: " can't serve traffic. Run"}}},
		{Kind: notification.CodeBlock, Spans: []notification.Span{
			{Text: "kubectl logs payments -n shop"}}},
	}
	m.Output = nil
	return m
}

// FixCommand is the command of ProFix's fix.
const FixCommand = "kubectl set resources deployment/payments -n shop " +
	"-c app --limits=memory=384Mi"

// ProFix is a Pro announcement whose proof is longer than any chat
// limit and that ends with a pinned fix: every provider with a length
// limit must keep the cause and the fix whole and cut the proof.
func ProFix() notification.Message {
	m := Announce()
	doc := []notification.Block{{Kind: notification.Para,
		Spans: []notification.Span{{Text: m.Short}}}}
	for i := 0; i < 60; i++ {
		doc = append(doc, notification.Block{Kind: notification.Bullet,
			Spans: []notification.Span{{Text: "Memory climbed from " +
				"118Mi to 255Mi before kill " + strings.Repeat("·", 60)}}})
	}
	doc = append(doc,
		notification.Block{Kind: notification.Para, Pinned: true,
			Spans: []notification.Span{{Text: "Raise the limit above " +
				"the observed peak (changes the cluster):"}}},
		notification.Block{Kind: notification.CodeBlock, Pinned: true,
			Spans: []notification.Span{{Text: FixCommand}}})
	m.Doc = doc
	m.Steps = []notification.Step{{Text: "Raise the limit",
		Command: FixCommand, Mutating: true}}
	m.Output = []string{strings.Repeat("cache warm: 41882 entries ", 40)}
	return m
}
