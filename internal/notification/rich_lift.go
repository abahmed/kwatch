package notification

import "strings"

// liftsCode reports a block with a Code span that the dialect cannot
// write inline exactly as it is.
func (d Dialect) liftsCode(b Block) bool {
	if d.SpanFits == nil {
		return false
	}
	for _, s := range b.Spans {
		if s.Style == Code && !d.SpanFits(s.Text) {
			return true
		}
	}
	return false
}

// renderLifted writes a line whose Code span does not fit inline: the
// words before it, the text in a Fence block, then the words after it.
// Without the lift the dialect would have to change the text.
func renderLifted(b Block, d Dialect) string {
	var pieces []string
	var line strings.Builder
	if b.Kind == Bullet {
		line.WriteString(d.Bullet)
	}
	flush := func() {
		text := line.String()
		line.Reset()
		if strings.Trim(text, " .,;:") != "" &&
			strings.TrimSpace(text) != strings.TrimSpace(d.Bullet) {
			pieces = append(pieces, strings.TrimSpace(text))
		}
	}
	for _, s := range b.Spans {
		if s.Style == Code && !d.SpanFits(s.Text) {
			flush()
			pieces = append(pieces, d.Fence(d.neutral(s.Text)))
			continue
		}
		if len(pieces) > 0 && line.Len() == 0 && s.Style == Plain {
			// The words after a block continue its sentence.
			s.Text = strings.TrimLeft(s.Text, " .,;:")
		}
		line.WriteString(renderSpan(s, b.Kind, d))
	}
	flush()
	return strings.Join(pieces, "\n")
}
