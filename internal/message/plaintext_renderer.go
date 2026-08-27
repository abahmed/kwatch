package message

// PlainTextRenderer produces unformatted text from a Report: the shared
// layout with no markup, for providers that render text verbatim.
type PlainTextRenderer struct{ textRenderer }

// NewPlainTextRenderer returns a PlainTextRenderer.
func NewPlainTextRenderer() *PlainTextRenderer {
	return &PlainTextRenderer{textRenderer{m: plainMarkup}}
}
