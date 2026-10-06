package notification

import "strings"

// Style is how a span of text is emphasised. It is a meaning, not a
// provider's syntax: each Dialect turns it into its own markup.
type Style uint8

// Span styles.
const (
	// Plain is kwatch's own wording.
	Plain Style = iota
	// Bold marks a resource name: a workload, namespace or node.
	Bold
	// Italic marks small print.
	Italic
	// Code marks text a pod wrote, or a command.
	Code
)

// Span is a run of text in one style. Text is always raw: the dialect
// escapes it, so text from a pod can never become markup.
type Span struct {
	Text  string
	Style Style
}

// BlockKind is the shape of one block of a message.
type BlockKind uint8

// Block kinds.
const (
	// Para is a line or paragraph.
	Para BlockKind = iota
	// Heading is a bold section title, set apart by a blank line.
	Heading
	// Bullet is one item of a list.
	Bullet
	// CodeBlock is a command or other verbatim text, in Spans[0].
	CodeBlock
	// Small is small print (italic where the provider has italics).
	Small
)

// Block is one line, item or code block of a message.
type Block struct {
	Kind  BlockKind
	Spans []Span
}

// Text is the block's text without any markup.
func (b Block) Text() string {
	var sb strings.Builder
	for _, s := range b.Spans {
		sb.WriteString(s.Text)
	}
	return sb.String()
}

// Dialect writes blocks in one provider's markup. Every func receives
// raw text and must return it escaped for that provider.
type Dialect struct {
	// Neutralize breaks mentions in raw text before it is escaped.
	Neutralize func(string) string
	// Text escapes plain text.
	Text func(string) string
	// Bold, Italic and Code wrap raw text in the style, escaping it.
	Bold, Italic, Code func(string) string
	// Fence wraps raw text as a code block.
	Fence func(string) string
	// Bullet starts a list item.
	Bullet string
	// Gap separates two paragraphs.
	Gap string
	// Ellipsis ends a cut message; empty means "…".
	Ellipsis string
}

// Blocks returns the blocks of the message. A message without structured
// blocks (an operator notice, a message read back from older state) is
// its Note as one paragraph.
func (m Message) Blocks() []Block {
	if len(m.Doc) > 0 {
		return m.Doc
	}
	return []Block{{Kind: Para,
		Spans: []Span{{Text: m.NoteParagraph()}}}}
}

// Render writes the message in the dialect.
func (m Message) Render(d Dialect) string {
	return RenderBlocks(m.Blocks(), d)
}

// RenderWithin writes the message in the dialect within limit bytes.
// Whole trailing blocks are dropped and replaced by an ellipsis line, so
// a cut never lands inside markup. limit 0 means no bound.
func (m Message) RenderWithin(d Dialect, limit int) string {
	return RenderBlocksWithin(m.Blocks(), d, limit)
}

// RenderBlocks writes blocks in the dialect.
func RenderBlocks(blocks []Block, d Dialect) string {
	return RenderBlocksWithin(blocks, d, 0)
}

// RenderBlocksWithin is RenderBlocks bounded to limit bytes (0: none).
func RenderBlocksWithin(blocks []Block, d Dialect, limit int) string {
	var sb strings.Builder
	prev := Block{Kind: Heading}
	for i, b := range blocks {
		piece := renderBlock(b, d)
		sep := ""
		if i > 0 {
			sep = separator(prev, b, d)
		}
		if limit > 0 && sb.Len()+len(sep)+len(piece) > limit {
			if i == 0 {
				return cut(plainOf(b, d), d, limit)
			}
			more := d.ellipsis()
			if sb.Len()+len(sep)+len(more) > limit {
				break
			}
			sb.WriteString(sep + more)
			break
		}
		sb.WriteString(sep + piece)
		prev = b
	}
	return sb.String()
}

// plainOf is the block's text escaped as plain text, the fallback for a
// first block too long to fit.
func plainOf(b Block, d Dialect) string {
	return d.Text(d.neutral(b.Text()))
}

func (d Dialect) neutral(s string) string {
	if d.Neutralize == nil {
		return s
	}
	return d.Neutralize(s)
}

// separator is what goes between two blocks.
func separator(prev, next Block, d Dialect) string {
	switch {
	case prev.Kind == Bullet && next.Kind == Bullet:
		return "\n"
	case next.Kind == Heading || prev.Kind == Bullet:
		return "\n\n"
	case prev.Kind == Heading:
		return "\n"
	default:
		return d.Gap
	}
}

func renderBlock(b Block, d Dialect) string {
	if b.Kind == CodeBlock {
		return d.Fence(d.neutral(b.Text()))
	}
	var sb strings.Builder
	if b.Kind == Bullet {
		sb.WriteString(d.Bullet)
	}
	for _, s := range b.Spans {
		sb.WriteString(renderSpan(s, b.Kind, d))
	}
	return sb.String()
}

func renderSpan(s Span, kind BlockKind, d Dialect) string {
	text := d.neutral(s.Text)
	style := s.Style
	if style == Plain {
		switch kind {
		case Heading:
			style = Bold
		case Small:
			style = Italic
		}
	}
	switch style {
	case Bold:
		return d.Bold(text)
	case Italic:
		return d.Italic(text)
	case Code:
		return d.Code(text)
	default:
		return d.Text(text)
	}
}

// AppendNote adds text after the narrative, as its own paragraph of the
// blocks and after sep in the Note.
func (m *Message) AppendNote(sep, text string) {
	if len(m.Doc) == 0 {
		m.Note = m.NoteText() + sep + text
		return
	}
	m.Note = m.NoteParagraph() + sep + text
	m.Doc = append(append([]Block(nil), m.Doc...),
		Block{Kind: Para, Spans: []Span{{Text: text}}})
}

// AppendMessage adds the narrative of other after the narrative of m.
func (m *Message) AppendMessage(sep string, other Message) {
	if len(m.Doc) == 0 {
		m.Note = m.NoteText() + sep + other.NoteText()
		return
	}
	m.Note = m.NoteParagraph() + sep + other.NoteParagraph()
	m.Doc = append(append([]Block(nil), m.Doc...), other.Blocks()...)
}

// WithMarker is the message with the status marker its narrative starts
// with replaced. A narrative without a leading marker is unchanged.
func (m Message) WithMarker(marker string) Message {
	m.Note = swapMarker(m.NoteParagraph(), marker)
	if len(m.Doc) == 0 || len(m.Doc[0].Spans) == 0 {
		return m
	}
	first := m.Doc[0].Spans[0]
	swapped := swapMarker(first.Text, marker)
	if swapped == first.Text {
		return m
	}
	doc := append([]Block(nil), m.Doc...)
	spans := append([]Span(nil), doc[0].Spans...)
	spans[0].Text = swapped
	doc[0].Spans = spans
	m.Doc = doc
	return m
}

// swapMarker replaces the status marker text starts with.
func swapMarker(text, marker string) string {
	for _, old := range Markers() {
		if strings.HasPrefix(text, old) {
			return marker + strings.TrimPrefix(text, old)
		}
	}
	return text
}

func (d Dialect) ellipsis() string {
	if d.Ellipsis == "" {
		return "…"
	}
	return d.Ellipsis
}

// cut shortens text to limit bytes, ending in the dialect's ellipsis.
func cut(text string, d Dialect, limit int) string {
	out := Truncate(text, limit)
	if out != text && d.Ellipsis != "" {
		out = strings.Replace(out, "…", d.Ellipsis, 1)
	}
	return out
}
