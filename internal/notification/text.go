package notification

import "strings"

// Text renders a message as plain sentences for providers without rich
// formatting. It is the Note when the message has one. Otherwise it
// writes the note the same way from the structured fields: the marker
// and title, the explanation lines and the first suggested step with
// its command. Like a Note, it has no labels, lists or links; the
// timeline and the application output stay with the providers that
// render them on their own.
func Text(m Message) string {
	if note := strings.TrimSpace(m.Note); note != "" {
		return note
	}
	var rest []string
	for _, line := range m.Lines {
		rest = append(rest, sentence(line))
	}
	if len(m.Steps) > 0 {
		rest = append(rest, stepSentence(m.Steps[0]))
	}
	lead := strings.TrimSpace(m.Title)
	if len(rest) > 0 {
		lead = sentence(lead)
	}
	parts := append([]string{m.markerOf(), lead}, rest...)
	return strings.TrimSpace(strings.Join(parts, " "))
}

// stepSentence writes a step as one sentence, its command after a colon,
// the way the composer writes the note's action.
func stepSentence(step Step) string {
	text := strings.TrimRight(strings.TrimSpace(step.Text), ".")
	if step.Command == "" {
		return sentence(text)
	}
	return text + ": " + step.Command
}

// sentence ends text with a full stop unless it already ends a sentence.
func sentence(text string) string {
	text = strings.TrimSpace(text)
	if text == "" || strings.ContainsAny(text[len(text)-1:], ".!?:") {
		return text
	}
	return text + "."
}
