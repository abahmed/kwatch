package reason

import (
	"strconv"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/signal"
)

func evidenceText(s signal.Signal) string {
	parts := []string{s.Summary}
	for _, e := range s.Evidence {
		parts = append(parts, e.Value)
	}
	return strings.Join(parts, " ")
}

// describeFields renders a change as "changed image api:v1 → api:v2".
func describeFields(change knowledge.Change) string {
	if len(change.Fields) == 0 {
		return "changed"
	}
	field := change.Fields[0]
	out := "changed " + lastSegment(field.Path)
	if field.Before != "" || field.After != "" {
		out += " " + orNone(field.Before) + " → " + orNone(field.After)
	}
	if extra := len(change.Fields) - 1; extra > 0 {
		out += " (+" + strconv.Itoa(extra) + " more)"
	}
	return out
}

func lastSegment(path string) string {
	if i := strings.LastIndexAny(path, ".]"); i >= 0 && i+1 < len(path) {
		return path[i+1:]
	}
	return path
}

func orNone(value string) string {
	if value == "" {
		return "none"
	}
	return value
}

func short(d time.Duration) string { return format.Duration(d) }

func containsFold(text, token string) bool {
	return strings.Contains(strings.ToLower(text), strings.ToLower(token))
}

func evidenceValue(s signal.Signal, label string) string {
	for _, e := range s.Evidence {
		if e.Label == label {
			return e.Value
		}
	}
	return ""
}
