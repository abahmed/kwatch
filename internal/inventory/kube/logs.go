package kube

import (
	"bufio"
	"bytes"
	"context"
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
	pod, name, ok := strings.Cut(container.Name, "/")
	if !ok || r.Client == nil {
		return nil
	}
	fetchCtx, cancel := context.WithTimeout(ctx, logFetchBudget)
	defer cancel()
	for _, previous := range []bool{true, false} {
		tail, limit := int64(logTailLines), int64(logFetchBytes)
		body, err := r.Client.CoreV1().Pods(container.Namespace).GetLogs(
			pod, &corev1.PodLogOptions{
				Container: name, Previous: previous,
				TailLines: &tail, LimitBytes: &limit,
			}).DoRaw(fetchCtx)
		if err == nil && len(bytes.TrimSpace(body)) > 0 {
			return logLines(lastBytes(body, logLimitBytes))
		}
	}
	return nil
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
	scanner := bufio.NewScanner(strings.NewReader(text))
	scanner.Buffer(make([]byte, 0, 4096), 64<<10)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !base64Line.MatchString(line) {
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
