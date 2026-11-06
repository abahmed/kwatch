package compose

import (
	"regexp"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
)

// A custom resource that reports a failing condition says why in the
// condition's message, and its controller often says it better in its
// newest Warning event. The state carries the most specific of them,
// never only the reason keyword: "ClusterIssuer ca is not ready:
// "contact email has forbidden domain …"" instead of "is not ready (err
// register ACME account)".

// genericDetail is a message that says something failed, not why.
var genericDetail = regexp.MustCompile(`(?i)^(could not|couldn't|` +
	`failed to|unable to|error) (get|read|sync|fetch|reconcile|` +
	`update|create)\b[^:]*$`)

// customDetail is the most specific text a custom resource's failing
// condition carries: the last clause of its message (after " :: ", the
// separator many controllers nest errors with, else after the last ": "
// when what comes before is a prefix of what failed), or its newest
// Warning event when the message only says that something failed.
func customDetail(f detection.Finding) string {
	if f.Reason != reasons.CustomResourceFailure {
		return ""
	}
	detail := specificClause(evidence(f, "message"))
	if detail == "" || genericDetail.MatchString(detail) {
		if event := specificClause(evidence(f, "event")); event != "" {
			return event
		}
	}
	return detail
}

// specificClause keeps the innermost part of a nested error message.
func specificClause(message string) string {
	message = strings.TrimSpace(message)
	if i := strings.LastIndex(message, " :: "); i >= 0 {
		return strings.TrimSpace(message[i+4:])
	}
	return message
}

// withCustomDetail replaces the reason keyword of a custom resource's
// state ("is not ready (err register ACME account)") with its detail.
func withCustomDetail(state string, f detection.Finding) string {
	detail := customDetail(f)
	if detail == "" {
		return state
	}
	if i := strings.Index(state, " ("); i > 0 {
		state = state[:i]
	}
	return state + ": " + quoted(detail)
}

// quotedByCustomDetail reports an error the lead quotes already as its
// custom resource's detail: the message says it once.
func quotedByCustomDetail(f caseFacts, said string) bool {
	if !f.ok {
		return false
	}
	detail := customDetail(f.lead)
	return detail != "" && strings.Contains(said, detail)
}
