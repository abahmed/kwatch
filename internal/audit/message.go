package audit

import "strings"

// Log levels of an entry, so log tools can colour and filter audit lines
// like any other record.
const (
	LevelInfo    = "info"
	LevelWarning = "warning"
	LevelError   = "error"
)

// maxMessage caps the one-line summary of an entry.
const maxMessage = 200

// LevelFor maps the tier an entry was delivered at to a log level: a
// page is an error, a notification a warning, and the quiet tiers (and
// entries without a tier) are information.
func LevelFor(tier string) string {
	switch tier {
	case "page":
		return LevelError
	case "notify":
		return LevelWarning
	}
	return LevelInfo
}

// Describe builds the short human line of an entry from its own fields:
// "deployment/web in prod: alert updated (still open)". It is
// deterministic and carries no wording of its own about causes.
func Describe(e Entry) string {
	var b strings.Builder
	if subject := firstNonEmpty(e.Subject, e.Root); subject != "" {
		if e.Namespace != "" {
			subject = strings.Replace(subject, "/"+e.Namespace+"/", "/", 1)
		}
		b.WriteString(subject)
		if e.Namespace != "" {
			b.WriteString(" in " + e.Namespace)
		}
		b.WriteString(": ")
	}
	b.WriteString(actionWords(e))
	if e.DecisionReason != "" {
		b.WriteString(" (" + e.DecisionReason + ")")
	}
	return capMessage(b.String())
}

func actionWords(e Entry) string {
	switch e.Delivery {
	case "dropped":
		return "left out of the digest"
	case "unannounced":
		return "recorded, not announced"
	}
	switch e.Action {
	case ActionCreate:
		return "alert sent"
	case ActionUpdate:
		return "alert updated"
	case ActionResolved:
		return "resolved"
	}
	return "summary sent"
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func capMessage(s string) string {
	r := []rune(s)
	if len(r) <= maxMessage {
		return s
	}
	return string(r[:maxMessage-3]) + "..."
}
