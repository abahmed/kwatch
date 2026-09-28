package story

import "strings"

// Text renders a message as plain text with light Markdown: the status
// marker and title, the story, a short timeline and the next steps. It is
// the fallback rendering for providers without rich formatting.
func Text(m Message) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(m.Status.Emoji() + " " + m.Title))
	b.WriteString("\n")
	for _, line := range m.Lines {
		b.WriteString(line)
		b.WriteString("\n")
	}
	if m.Confidence != "" {
		b.WriteString("Confidence: " + m.Confidence + "\n")
	}
	if len(m.Timeline) > 0 {
		b.WriteString("\nTimeline (UTC):\n")
		for _, event := range m.Timeline {
			b.WriteString("  " + event + "\n")
		}
	}
	if len(m.Steps) > 0 {
		b.WriteString("\nNext steps:\n")
		for _, step := range m.Steps {
			b.WriteString("• " + step.Text + "\n")
			if step.Command != "" {
				b.WriteString("  " + step.Command + "\n")
			}
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
