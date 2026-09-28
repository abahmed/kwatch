package story

// Message is a provider-neutral story. Renderers turn it into plain text,
// Markdown, Slack blocks or HTML.
type Message struct {
	// Key identifies the conversation, so providers can edit or thread.
	Key string
	// Revision orders messages of one conversation.
	Revision int
	Status   Status
	// Title is one line: what is wrong and where.
	Title string
	// Lines are the story sentences, in reading order.
	Lines []string
	// Timeline lists the relevant events, oldest first.
	Timeline []string
	// Steps are commands or actions for the reader.
	Steps []Step
	// Confidence describes how sure the cause is, when there is one.
	Confidence string
}

// Status drives the single status marker a renderer shows.
type Status uint8

// Statuses.
const (
	StatusCritical Status = iota + 1
	StatusWarning
	StatusFlapping
	StatusResolved
)

// Emoji is the one status marker for a status.
func (s Status) Emoji() string {
	switch s {
	case StatusCritical:
		return "🔴"
	case StatusWarning:
		return "🟠"
	case StatusFlapping:
		return "🔁"
	case StatusResolved:
		return "✅"
	default:
		return ""
	}
}

// Step is one suggested action. Mutating steps change the cluster and are
// labelled as such.
type Step struct {
	Text     string
	Command  string
	Mutating bool
}
