//go:build e2e

package harness

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// auditPollInterval is how often a wait re-reads the Kwatch log. Scenarios
// run in parallel and each one reads the whole log, so this stays modest.
const auditPollInterval = 2 * time.Second

type AuditReader struct {
	environment *Environment
}

type AuditMatch struct {
	// Incident, when set, matches only entries of that incident ID.
	Incident  string
	Namespace string
	Resource  string
	Reason    string
	Action    string
	Count     int
}

func NewAuditReader(environment *Environment) *AuditReader {
	return &AuditReader{environment: environment}
}

func (a *AuditReader) WaitFor(
	ctx context.Context,
	match AuditMatch,
) ([]AuditEntry, error) {
	deadline, cancel := withDefaultDeadline(ctx, 10*time.Minute)
	defer cancel()
	ticker := time.NewTicker(auditPollInterval)
	defer ticker.Stop()
	for {
		entries, err := a.snapshot(deadline)
		if err == nil {
			matched := matchingEntries(entries, match)
			if len(matched) >= match.Count {
				return matched, nil
			}
		}
		select {
		case <-deadline.Done():
			return nil, fmt.Errorf("wait for audit match: %w", deadline.Err())
		case <-ticker.C:
		}
	}
}

func (a *AuditReader) snapshot(ctx context.Context) ([]AuditEntry, error) {
	pods, err := a.environment.Client.CoreV1().Pods(
		a.environment.Config.KwatchNamespace,
	).List(ctx, metav1.ListOptions{LabelSelector: "app=kwatch"})
	if err != nil {
		return nil, err
	}
	var result []AuditEntry
	for _, pod := range pods.Items {
		output, err := a.logs(ctx, pod.Name)
		if err != nil {
			continue
		}
		result = append(result, parseAudit(output)...)
	}
	return result, nil
}

func (a *AuditReader) logs(ctx context.Context, pod string) ([]byte, error) {
	request := a.environment.Client.CoreV1().Pods(
		a.environment.Config.KwatchNamespace,
	).GetLogs(pod, &corev1.PodLogOptions{})
	stream, err := request.Stream(ctx)
	if err != nil {
		return nil, err
	}
	defer stream.Close()
	return io.ReadAll(stream)
}

func parseAudit(payload []byte) []AuditEntry {
	var result []AuditEntry
	scanner := bufio.NewScanner(bytes.NewReader(payload))
	for scanner.Scan() {
		var entry AuditEntry
		if json.Unmarshal(scanner.Bytes(), &entry) == nil {
			result = append(result, entry)
		}
	}
	return result
}

func matchingEntries(entries []AuditEntry, match AuditMatch) []AuditEntry {
	result := make([]AuditEntry, 0, len(entries))
	for _, entry := range entries {
		if match.Incident != "" && entry.Incident != match.Incident {
			continue
		}
		if match.Namespace != "" && entry.Namespace != match.Namespace {
			continue
		}
		if match.Resource != "" && !resourceMatches(entry, match) {
			continue
		}
		if match.Reason != "" && !reasonMatches(entry, match) {
			continue
		}
		if match.Action != "" && entry.Action != match.Action {
			continue
		}
		result = append(result, entry)
	}
	return result
}

// resourceMatches compares the audit root ("Kind/namespace/name") with the
// scenario resource. A root that is a Pod owned by the resource
// (name-hash-suffix) or a container of the Pod ("pod/container") counts as
// the resource, because the incident may be rooted at the failing Pod, at
// one of its containers or at its owner.
func resourceMatches(entry AuditEntry, match AuditMatch) bool {
	name := entry.Name
	if name == "" {
		name = rootName(entry.Root)
	}
	if name == match.Resource {
		return true
	}
	if entry.Namespace != "" &&
		entry.Namespace+"/"+name == match.Resource {
		return true
	}
	return entry.Root != "" && (strings.HasPrefix(name, match.Resource+"-") ||
		strings.HasPrefix(name, match.Resource+"/"))
}

// reasonMatches accepts the exact reason list or any one reason in it. The
// audit reason joins every finding reason of the incident with commas.
func reasonMatches(entry AuditEntry, match AuditMatch) bool {
	if entry.Reason == match.Reason {
		return true
	}
	for _, reason := range strings.Split(entry.Reason, ",") {
		if reason == match.Reason {
			return true
		}
	}
	return false
}

func rootName(root string) string {
	parts := strings.SplitN(root, "/", 3)
	if len(parts) != 3 {
		return root
	}
	return parts[2]
}

// AssertRoot polls the audit log until an in-scope incident has the
// expected root, then requires the complete verdict (tier, message budget,
// must-not-blame) to be clean. It is bounded by ctx.
func (a *AuditReader) AssertRoot(
	ctx context.Context, exp RootExpectation, scope RootScope,
) error {
	deadline, cancel := withDefaultDeadline(ctx, 10*time.Minute)
	defer cancel()
	ticker := time.NewTicker(auditPollInterval)
	defer ticker.Stop()
	var verdict RootVerdict
	for {
		if entries, err := a.snapshot(deadline); err == nil {
			verdict = EvaluateRoot(entries, exp, scope)
			if verdict.Rooted {
				return verdict.Err()
			}
		}
		select {
		case <-deadline.Done():
			return fmt.Errorf("wait for root %q: %w: %v",
				exp.Root, deadline.Err(), verdict.Problems)
		case <-ticker.C:
		}
	}
}

// withDefaultDeadline keeps the caller's deadline when it has one, so a
// scenario can wait longer than the default, and adds limit otherwise.
func withDefaultDeadline(
	ctx context.Context, limit time.Duration,
) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, limit)
}
