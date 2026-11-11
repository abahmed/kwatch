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

const (
	// maxAuditLineBytes is the longest log line parseAudit accepts; the
	// default scanner stops at 64 KiB and would drop every later entry.
	maxAuditLineBytes = 4 << 20
	// maxAuditLogBytes bounds one pod's log read on each poll.
	maxAuditLogBytes = 256 << 20
)

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
	// DecisionReason, when set, matches only entries sent for that
	// reason, such as FailingAgain.
	DecisionReason string
	Count          int
}

func NewAuditReader(environment *Environment) *AuditReader {
	return &AuditReader{environment: environment}
}

func (a *AuditReader) WaitFor(
	ctx context.Context,
	match AuditMatch,
) ([]AuditEntry, error) {
	return a.WaitForAny(ctx, match)
}

// WaitForAny waits until one of matches is met and returns its entries.
// A scenario uses it when Kwatch may tell the same thing in more than
// one way, such as a returning failure that opens a new incident or
// reopens the old one.
func (a *AuditReader) WaitForAny(
	ctx context.Context,
	matches ...AuditMatch,
) ([]AuditEntry, error) {
	deadline, cancel := withDefaultDeadline(ctx, 10*time.Minute)
	defer cancel()
	ticker := time.NewTicker(auditPollInterval)
	defer ticker.Stop()
	for {
		entries, err := a.snapshot(deadline)
		if err == nil {
			if matched, ok := firstMet(entries, matches); ok {
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

// Matching returns the audit entries that match m as of now, without
// waiting. A scenario uses it to show that something was not announced.
func (a *AuditReader) Matching(
	ctx context.Context, m AuditMatch,
) ([]AuditEntry, error) {
	entries, err := a.snapshot(ctx)
	if err != nil {
		return nil, err
	}
	return matchingEntries(entries, m), nil
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
		entries, parseErr := parseAudit(output)
		if parseErr != nil {
			return nil, fmt.Errorf("read audit log of %s: %w",
				pod.Name, parseErr)
		}
		result = append(result, entries...)
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
	return io.ReadAll(io.LimitReader(stream, maxAuditLogBytes))
}

func parseAudit(payload []byte) ([]AuditEntry, error) {
	var result []AuditEntry
	scanner := bufio.NewScanner(bytes.NewReader(payload))
	scanner.Buffer(make([]byte, 0, 64<<10), maxAuditLineBytes)
	for scanner.Scan() {
		var entry AuditEntry
		if json.Unmarshal(scanner.Bytes(), &entry) == nil {
			result = append(result, entry)
		}
	}
	return result, scanner.Err()
}

// ToldAgain lists the two ways Kwatch tells that the failure of an
// incident it resolved came back: the old incident reopens under its
// own ID with a "failing again" update, or a second incident of the
// same resource opens. Which one happens is Kwatch's choice, not
// timing the scenario controls: the old incident reopens only while
// the returning failure has the root the incident resolved with, and
// a cause revision while it recovered may have moved that root.
func ToldAgain(
	namespace, incident, resource, reason string,
) []AuditMatch {
	return []AuditMatch{
		{Incident: incident, Action: "update", Reason: reason,
			DecisionReason: FailingAgain, Count: 1},
		{Namespace: namespace, Resource: resource, Reason: reason,
			Action: "create", Count: 2},
	}
}

// firstMet returns the entries of the first match that has at least
// Count entries (at least one when Count is 0).
func firstMet(
	entries []AuditEntry, matches []AuditMatch,
) ([]AuditEntry, bool) {
	for _, match := range matches {
		matched := matchingEntries(entries, match)
		if len(matched) >= max(match.Count, 1) {
			return matched, true
		}
	}
	return nil, false
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
		if match.DecisionReason != "" &&
			entry.DecisionReason != match.DecisionReason {
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
