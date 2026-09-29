package delivery

import (
	"bytes"
	"text/template"
	"unicode/utf8"

	"github.com/abahmed/kwatch/internal/notice"
)

// templateData is what a user's per-reason message template can use: the
// structured message and its standard text rendering.
type templateData struct {
	Message notice.Message
	Text    string
}

// storyText renders a story, applying the first user template whose
// reason the story contains. A template that fails falls back to the
// standard text, so a template mistake never loses a message.
func storyText(
	m notice.Message, templates map[string]*template.Template,
) string {
	text := notice.Text(m)
	for _, reason := range m.Route.Reasons {
		t, ok := templates[lowerASCII(reason)]
		if !ok {
			continue
		}
		var buf bytes.Buffer
		if err := t.Execute(&buf, templateData{
			Message: m, Text: text,
		}); err == nil && buf.Len() > 0 {
			return buf.String()
		}
	}
	return text
}

func lowerASCII(value string) string {
	out := []byte(value)
	for i, c := range out {
		if c >= 'A' && c <= 'Z' {
			out[i] = c + 'a' - 'A'
		}
	}
	return string(out)
}

func truncateMsg(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if len(s) <= maxLen {
		return s
	}
	suffix := "\n…(truncated)"
	cut := maxLen - len(suffix)
	if cut <= 0 {
		return suffix
	}
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + suffix
}

func defaultMaxBytes(providerName string) int {
	return providerPayloadPolicy(providerName).maxBytes
}
