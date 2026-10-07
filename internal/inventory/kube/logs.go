package kube

import (
	"bytes"
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/redact"
)

const (
	logTailLines = 200
	// crashTailLines and crashKeepBytes bound the previous-run read that
	// looks for the crash's first error line.
	crashTailLines = 50
	crashKeepBytes = 16 << 10
	// logFetchBytes is asked of the API server, logLimitBytes is kept.
	// The server applies LimitBytes to the start of the tail, so a tail
	// over the limit loses its end, where the crash message is; kwatch
	// asks for more and keeps the end itself.
	logFetchBytes  = 256 << 10
	logLimitBytes  = 64 << 10
	logFetchBudget = 8 * time.Second
	maxLogExcerpt  = 5
)

// errorLine matches lines that usually carry the failure, including
// named errors such as "ValueError:" or "IllegalStateException".
var errorLine = regexp.MustCompile(`(?i)\b(error|fatal|panic|exception|` +
	`failed|refused|timeout|denied|cannot|unable|traceback|oomkilled)\b|` +
	`\w(error|exception)\b`)

// base64Line matches a line that is nothing but a long base64 run, such as
// the body of a key whose BEGIN line fell outside the log tail.
var base64Line = regexp.MustCompile(`^[A-Za-z0-9+/_-]{40,}={0,2}$`)

// runtimeLogFailure matches what the kubelet or container runtime writes
// into the log body when it cannot read the container's log file. It is
// the node's error, not the application's, so it is never output.
var runtimeLogFailure = regexp.MustCompile(`(?i)^(` +
	`unable to retrieve container logs for |` +
	`failed to try resolving symlinks in path|` +
	`rpc error: .*(container|task) .*not found|` +
	`container \S+ not found|` +
	`failed to get container logs? )`)

// IsRuntimeLogFailure reports text that is the kubelet or runtime failing
// to read a container's log, not anything the application said.
func IsRuntimeLogFailure(text string) bool {
	return runtimeLogFailure.MatchString(strings.TrimSpace(text))
}

// IsErrorLine reports whether a log line looks like it carries a failure.
func IsErrorLine(line string) bool {
	return errorLine.MatchString(line)
}

// LogReader fetches a container's recent output for evidence.
type LogReader struct {
	Client kubernetes.Interface
}

// Excerpt returns the most telling lines of a container's previous (or,
// when it has not restarted, current) output: error-looking lines, else
// the last lines. Output is redacted before it leaves this function.
func (r LogReader) Excerpt(
	ctx context.Context, container inventory.EntityID,
) []string {
	return ErrorExcerpt(r.Lines(ctx, container))
}

// Lines returns the recent output of a container's previous run, or of
// its current run when it has not restarted, oldest first. Every line is
// redacted and bounded; blank lines are dropped. It reads at most
// logTailLines lines within logFetchBudget and keeps their last
// logLimitBytes bytes.
func (r LogReader) Lines(
	ctx context.Context, container inventory.EntityID,
) []string {
	for _, previous := range []bool{true, false} {
		body, err := r.read(ctx, container, previous, logTailLines, logFetchBytes)
		if err == nil && len(bytes.TrimSpace(body)) > 0 {
			return logLines(lastBytes(body, logLimitBytes))
		}
	}
	return nil
}

// CurrentLines returns the recent output of a container's current run
// only, oldest first, redacted and bounded like Lines. It is for a
// container that is still running, such as a stuck init container.
func (r LogReader) CurrentLines(
	ctx context.Context, container inventory.EntityID,
) []string {
	body, err := r.read(ctx, container, false, crashTailLines, logFetchBytes)
	if err != nil {
		return nil
	}
	return logLines(lastBytes(body, crashKeepBytes))
}

// PreviousLines returns the output of a container's previous run only,
// never of the current one, as Lines would. An empty result with a nil
// error means the previous run printed nothing; an error means the
// read failed (a Forbidden one means pods/log is not granted).
func (r LogReader) PreviousLines(
	ctx context.Context, container inventory.EntityID,
) ([]string, error) {
	body, err := r.read(ctx, container, true, crashTailLines, logFetchBytes)
	if err != nil {
		return nil, err
	}
	return logLines(lastBytes(body, crashKeepBytes)), nil
}

// read fetches one run's log tail within logFetchBudget.
func (r LogReader) read(
	ctx context.Context, container inventory.EntityID, previous bool,
	lines, fetch int64,
) ([]byte, error) {
	pod, name, ok := strings.Cut(container.Name, "/")
	if !ok || r.Client == nil {
		return nil, errors.New("no log client or container name")
	}
	fetchCtx, cancel := context.WithTimeout(ctx, logFetchBudget)
	defer cancel()
	tail, limit := lines, fetch
	return r.Client.CoreV1().Pods(container.Namespace).GetLogs(
		pod, &corev1.PodLogOptions{
			Container: name, Previous: previous,
			TailLines: &tail, LimitBytes: &limit,
		}).DoRaw(fetchCtx)
}

// lastBytes keeps at most the last n bytes of body, from the first
// whole line within them, or from a rune boundary when one line is
// longer than n.
func lastBytes(body []byte, n int) []byte {
	if len(body) <= n {
		return body
	}
	start := len(body) - n
	if body[start-1] == '\n' {
		return body[start:]
	}
	tail := body[start:]
	if i := bytes.IndexByte(tail, '\n'); i >= 0 {
		return tail[i+1:]
	}
	for len(tail) > 0 && !utf8.RuneStart(tail[0]) {
		tail = tail[1:]
	}
	return tail
}

// logLines splits a log body into redacted, bounded, non-blank lines.
// The whole body is redacted before splitting, because some secrets (a
// PEM private key) span several lines and per-line redaction would leak
// their middle. Lines that are only a long base64 run are dropped too.
func logLines(body []byte) []string {
	var lines []string
	text := redact.Evidence(string(body))
	// Split rather than scan: a scanner stops for good at its first line
	// over its token limit (redaction can lengthen one past the body's
	// own limit) and would silently drop the rest of the excerpt.
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !base64Line.MatchString(line) &&
			!IsRuntimeLogFailure(line) {
			lines = append(lines, truncate(line))
		}
	}
	return lines
}

// ErrorExcerpt keeps the error-looking lines, else every line, and at
// most the last maxLogExcerpt of them.
func ErrorExcerpt(all []string) []string {
	var errors []string
	for _, line := range all {
		if IsErrorLine(line) {
			errors = append(errors, line)
		}
	}
	lines := errors
	if len(lines) == 0 {
		lines = all
	}
	if len(lines) > maxLogExcerpt {
		lines = lines[len(lines)-maxLogExcerpt:]
	}
	return lines
}
