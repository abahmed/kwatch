package email

import (
	"html"
	"strings"

	"github.com/abahmed/kwatch/internal/notification"
)

// Evidence mail colours: text in Ink, mono blocks on Fog. Mail clients
// decide dark mode themselves.
const (
	inkColor = "#0E1116"
	fogColor = "#F2F3F0"
)

// evidenceText is a Pro message as plain text in the spec's order: the
// cause, what kwatch found, the log quote and the fix.
func evidenceText(m notification.Message) string {
	ev := m.Evidence
	var b strings.Builder
	b.WriteString(m.ShortText() + "\n")
	if len(ev.Facts) > 0 {
		b.WriteString("\nWhat kwatch found\n")
		for _, f := range ev.Facts {
			b.WriteString("- " + f + "\n")
		}
	}
	if q := ev.Quote; q != nil && len(q.Lines) > 0 {
		b.WriteString("\n" + quoteLabel(q) + "\n")
		for _, l := range q.Lines {
			b.WriteString("  " + l + "\n")
		}
	}
	if len(ev.Fix) > 0 {
		b.WriteString("\nFix (changes the cluster)\n")
		for _, s := range ev.Fix {
			b.WriteString(s.Text + "\n")
			if s.Command != "" {
				b.WriteString("  " + s.Command + "\n")
			}
		}
	}
	return b.String()
}

// evidenceHTML is the same content as HTML: every value is escaped and
// mono blocks sit on Fog.
func evidenceHTML(m notification.Message) string {
	ev := m.Evidence
	var b strings.Builder
	b.WriteString(`<!doctype html><html><head><meta charset="utf-8">` +
		`<meta name="color-scheme" content="light dark"></head>` +
		`<body style="font-family:sans-serif;color:` + inkColor + `">`)
	b.WriteString(`<p style="font-size:20px;font-weight:600">` +
		html.EscapeString(m.ShortText()) + `</p>`)
	if len(ev.Facts) > 0 {
		b.WriteString(`<h3>What kwatch found</h3><ul>`)
		for _, f := range ev.Facts {
			b.WriteString(`<li>` + html.EscapeString(f) + `</li>`)
		}
		b.WriteString(`</ul>`)
	}
	if q := ev.Quote; q != nil && len(q.Lines) > 0 {
		b.WriteString(`<h3>` + html.EscapeString(quoteLabel(q)) + `</h3>`)
		b.WriteString(mono(strings.Join(q.Lines, "\n")))
	}
	if len(ev.Fix) > 0 {
		b.WriteString(`<h3>Fix (changes the cluster)</h3>`)
		for _, s := range ev.Fix {
			b.WriteString(`<p>` + html.EscapeString(s.Text) + `</p>`)
			if s.Command != "" {
				b.WriteString(mono(s.Command))
			}
		}
	}
	b.WriteString(`</body></html>`)
	return b.String()
}

func mono(text string) string {
	return `<pre style="font-family:monospace;background:` + fogColor +
		`;padding:12px;white-space:pre-wrap">` + html.EscapeString(text) +
		`</pre>`
}

func quoteLabel(q *notification.Quote) string {
	label := "Log"
	if q.Container != "" {
		label += ", container " + q.Container
	}
	if q.Run != "" {
		label += ", " + q.Run + " run"
	}
	return label
}
