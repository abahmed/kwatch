package kube

import (
	"bufio"
	"bytes"
	"context"
	"regexp"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/message"
)

const (
	logTailLines   = 200
	logLimitBytes  = 64 << 10
	logFetchBudget = 8 * time.Second
	maxLogExcerpt  = 5
)

// errorLine matches lines that usually carry the failure.
var errorLine = regexp.MustCompile(`(?i)\b(error|fatal|panic|exception|` +
	`failed|refused|timeout|denied|cannot|unable|traceback|oomkilled)\b`)

// LogReader fetches a container's recent output for evidence.
type LogReader struct {
	Client kubernetes.Interface
}

// Excerpt returns the most telling lines of a container's previous (or,
// when it has not restarted, current) output: error-looking lines, else
// the last lines. Output is redacted before it leaves this function.
func (r LogReader) Excerpt(
	ctx context.Context, container knowledge.EntityID,
) []string {
	pod, name, ok := strings.Cut(container.Name, "/")
	if !ok || r.Client == nil {
		return nil
	}
	fetchCtx, cancel := context.WithTimeout(ctx, logFetchBudget)
	defer cancel()
	for _, previous := range []bool{true, false} {
		tail, limit := int64(logTailLines), int64(logLimitBytes)
		body, err := r.Client.CoreV1().Pods(container.Namespace).GetLogs(
			pod, &corev1.PodLogOptions{
				Container: name, Previous: previous,
				TailLines: &tail, LimitBytes: &limit,
			}).DoRaw(fetchCtx)
		if err == nil && len(bytes.TrimSpace(body)) > 0 {
			return excerpt(body)
		}
	}
	return nil
}

func excerpt(body []byte) []string {
	var all, errors []string
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 0, 4096), 64<<10)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		line = message.RedactEvidence(truncate(line))
		all = append(all, line)
		if errorLine.MatchString(line) {
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
