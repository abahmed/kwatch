//go:build e2e

package harness

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type AuditReader struct {
	environment *Environment
}

type AuditMatch struct {
	Namespace string
	Resource  string
	Reason    string
	Action    string
	Count     int
}

type AuditEntry struct {
	Timestamp   time.Time `json:"ts"`
	Action      string    `json:"action"`
	IncidentKey string    `json:"incidentKey"`
	IncidentID  string    `json:"id"`
	Namespace   string    `json:"namespace"`
	Reason      string    `json:"reason"`
	Name        string    `json:"name"`
	Count       int       `json:"count"`
}

func NewAuditReader(environment *Environment) *AuditReader {
	return &AuditReader{environment: environment}
}

func (a *AuditReader) WaitFor(
	ctx context.Context,
	match AuditMatch,
) ([]AuditEntry, error) {
	deadline, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	ticker := time.NewTicker(500 * time.Millisecond)
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
		if match.Namespace != "" && entry.Namespace != match.Namespace {
			continue
		}
		if match.Resource != "" && entry.Name != match.Resource {
			continue
		}
		if match.Reason != "" && entry.Reason != match.Reason {
			continue
		}
		if match.Action != "" && entry.Action != match.Action {
			continue
		}
		result = append(result, entry)
	}
	return result
}
