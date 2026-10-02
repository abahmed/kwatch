package notification

// Status markers. A narrative note starts with exactly one of them and
// carries no other emoji.
const (
	MarkerPage     = "🔴"
	MarkerNotify   = "🟠"
	MarkerLow      = "🟡"
	MarkerResolved = "✅"
)

// Markers lists every status marker, loudest first.
func Markers() []string {
	return []string{MarkerPage, MarkerNotify, MarkerLow, MarkerResolved}
}
