package providertest

import (
	"strings"

	"github.com/abahmed/kwatch/internal/notification"
)

// ContractCase is one canonical message of the provider contract.
type ContractCase struct {
	Name    string
	Message notification.Message
	// Lifecycle marks the three steps that must share one key.
	Lifecycle bool
}

// Canonical returns the fixed messages every provider contract renders:
// the page-tier open, a notify update and the resolve of one incident,
// a digest, markdown with code, unicode, and a message near limit bytes.
func Canonical(limit int) []ContractCase {
	if limit <= 0 {
		limit = 3000
	}
	return []ContractCase{
		{Name: "page-open", Message: Announce(), Lifecycle: true},
		{Name: "notify-update", Message: Update(), Lifecycle: true},
		{Name: "resolve", Message: Resolve(), Lifecycle: true},
		{Name: "digest", Message: Summary()},
		{Name: "markdown-code", Message: withKey(Rich(), "p-md")},
		{Name: "unicode", Message: unicode()},
		{Name: "near-limit", Message: nearLimit(limit)},
	}
}

func withKey(m notification.Message, key string) notification.Message {
	m.Key = key
	return m
}

func unicode() notification.Message {
	m := withKey(Announce(), "p-uni")
	m.Title = "платежи упали в магазине"
	m.Short = "🔴 сервис «платежи» в shop падает — " +
		"支付失败 ñandú."
	m.Note = m.Short +
		" Причина: OOMKilled 💥 (café, naïve, 日本語)."
	m.Output = []string{
		"паника: нехватка памяти — 内存不足",
	}
	m.Doc = nil
	return m
}

// nearLimit fills the note with fenced code lines so the whole text is
// about limit bytes; providers must shorten it without an open fence.
func nearLimit(limit int) notification.Message {
	m := withKey(Announce(), "p-big")
	head := "🔴 payments in shop is crash-looping. Last output:\n"
	line := strings.Repeat("x", 59) + "\n"
	fill := strings.Repeat(line, (limit-len(head))/len(line))
	m.Note = head + "```\n" + fill + "```"
	m.Short = "🔴 payments in shop is crash-looping."
	m.Doc = []notification.Block{
		{Kind: notification.Para, Spans: []notification.Span{
			{Text: head}}},
		{Kind: notification.CodeBlock, Spans: []notification.Span{
			{Text: strings.TrimSpace(fill)}}},
	}
	return m
}
