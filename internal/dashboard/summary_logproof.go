package dashboard

import (
	"regexp"
	"slices"
	"strings"
)

// errorLine matches a log line that reports a failure, including an
// application that says its own health check fails ("unhealthy", "not
// ready", a 5xx status it answered).
var errorLine = regexp.MustCompile(`(?i)\b(error|err|panic|fatal|` +
	`exception|fail(ed|ure)?|refused|denied|timeout|timed out|` +
	`cannot|can't|unable|traceback|killed|unhealthy|not ready)\b|` +
	`\w(error|exception)\b|` +
	`\b(status|code|http)[ =:/]*5\d\d\b`)

// scaffoldLine matches the lines a stack trace is made of around the
// line that states the error: frames ("at x", `File "x"`), the
// markers between exceptions (".NET: --- End of inner exception stack
// trace ---"), the Python and Go headers and Java's "... 3 more".
var scaffoldLine = regexp.MustCompile(`^(at\s|---(\s|$)|-{4,}|` +
	`Traceback \(|File "|goroutine \d|\.{3}\s*\d+ (more|common)|` +
	`\[?stack ?trace)`)

// causeLine matches a line that names the exception an outer one
// wraps: .NET "---> Inner: message", Java "Caused by: x".
var causeLine = regexp.MustCompile(`^(--->\s*|Caused by:\s*)`)

// logProof lists the captured log lines that report a failure,
// best first: the innermost cause of a wrapped exception, the line
// that names the error rather than the stack trace around it, then
// the newest of the rest.
func logProof(d *IncidentDetail) []SummaryProof {
	var plain, causes []string
	for _, line := range slices.Backward(d.CapturedLogLines) {
		line = strings.TrimSpace(line)
		switch {
		case line == "" || !errorLine.MatchString(line) ||
			scaffoldLine.MatchString(line):
		case causeLine.MatchString(line):
			causes = append(causes, causeLine.ReplaceAllString(line, ""))
		default:
			plain = append(plain, line)
		}
	}
	out := make([]SummaryProof, 0, len(causes)+len(plain))
	for _, line := range append(causes, plain...) {
		out = append(out, SummaryProof{Kind: "log",
			Text: "Last error in the logs", Detail: line})
	}
	return out
}
