package delivery

import (
	"errors"
	"regexp"
	"unicode/utf8"

	"k8s.io/klog/v2"
)

// maxLoggedError bounds the error text of a send log line.
const maxLoggedError = 200

// urlPattern matches the URLs a transport error may carry. Webhook URLs
// hold their secret in the path, so none is ever logged.
var urlPattern = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9+.-]*://\S+`)

// sendRecord is what one provider delivery attempt left behind, for the
// one log line that lets an operator verify delivery from the logs.
type sendRecord struct {
	provider  string
	key       string
	kind      string
	placement string
	retries   int
	err       error
	fallback  string
}

// logSend writes the outcome of one provider delivery: provider, incident
// key, message kind, placement, ok or error and how many retries it took.
// The message body is never logged.
func logSend(r sendRecord) {
	result := "ok"
	if r.err != nil {
		result = "error"
	}
	pairs := []any{
		"component", "delivery", "provider", r.provider, "key", r.key,
		"kind", r.kind, "placement", r.placement, "result", result,
		"retries", r.retries,
	}
	if r.err != nil {
		pairs = append(pairs, "error", loggedError(r.err))
	}
	if r.fallback != "" {
		pairs = append(pairs, "fallbackFor", r.fallback)
	}
	klog.InfoS("provider send", pairs...)
}

// loggedError is the error text without URLs, cut to a bounded length.
// The cut is on a character boundary, so the text stays valid UTF-8.
func loggedError(err error) string {
	text := urlPattern.ReplaceAllString(err.Error(), "<url>")
	if len(text) > maxLoggedError {
		cut := maxLoggedError
		for cut > 0 && !utf8.RuneStart(text[cut]) {
			cut--
		}
		text = text[:cut] + "..."
	}
	return text
}

// loggedErr is err as every log line of delivery should carry it: the
// same scrubbed text as the send log. A provider SDK error can quote the
// webhook URL, whose path is a secret.
func loggedErr(err error) error {
	if err == nil {
		return nil
	}
	return errors.New(loggedError(err))
}

// describeJob names what a job carries. The placement is derived from the
// message alone: the opening message is a root, a resolve edits the root
// (and replies in its thread), every other update is a thread reply.
// Providers without threads simply deliver the message.
func describeJob(job deliverJob) (kind, placement string) {
	m := job.incident
	if job.kind != jobIncident || m == nil {
		return "message", "root"
	}
	switch {
	case m.IsSummary():
		return "summary", "root"
	case m.Resolved():
		return "resolve", "edit"
	case m.Opens || m.Revision <= 1:
		return "incident", "root"
	}
	return "incident", "thread"
}

// newSendRecord builds the log record of one delivery attempt.
func newSendRecord(
	provider string, job deliverJob, opts deliverOpts, attempts int,
	err error,
) sendRecord {
	kind, placement := describeJob(job)
	return sendRecord{
		provider: provider, key: job.key(), kind: kind,
		placement: placement, retries: max(attempts-1, 0), err: err,
		fallback: opts.fallbackFrom,
	}
}
