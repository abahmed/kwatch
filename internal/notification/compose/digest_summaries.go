package compose

import "strings"

// Bounds of the digest lines the audit log quotes.
const (
	maxSummaryLines = 6
	maxSummaryRunes = 200
)

// summaryLines are the lines of a digest that stand for several
// problems or for the cluster, for the audit log: the wake-up line, the
// grouped lines and the unchanged line. The per-incident titles are
// left out, as the audit entry names the incidents itself.
func summaryLines(wake *WakeLine, list folded) []string {
	var out []string
	if wake != nil {
		out = append(out, wake.Text())
	}
	out = append(out, list.full...)
	out = append(out, list.same...)
	for i, line := range out {
		if r := []rune(strings.TrimSpace(line)); len(r) > maxSummaryRunes {
			line = string(r[:maxSummaryRunes]) + "..."
		}
		out[i] = line
	}
	return out[:min(len(out), maxSummaryLines)]
}
