package k8s

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"sort"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

// GetPodEventsStr returns formatted events as a string for specified pod
// MaxEventsInMessage bounds how many events are rendered into an alert. A
// pod that churns can accumulate hundreds, and an unbounded list pushes the
// message past the provider's block/length limits, which loses the whole
// alert rather than just the surplus events.
const MaxEventsInMessage = 40

// eventTime resolves the most meaningful timestamp an event carries.
func eventTime(ev v1.Event) time.Time {
	for _, ts := range []time.Time{
		ev.LastTimestamp.Time,
		ev.EventTime.Time,
		ev.FirstTimestamp.Time,
		ev.CreationTimestamp.Time,
	} {
		if !ts.IsZero() {
			return ts
		}
	}
	return time.Time{}
}

func GetPodEventsStr(events *[]v1.Event) string {
	if events == nil {
		return ""
	}

	// Oldest first, so trimming keeps the most recent events — the ones that
	// explain the current state.
	sorted := make([]v1.Event, len(*events))
	copy(sorted, *events)
	sort.SliceStable(sorted, func(i, j int) bool {
		return eventTime(sorted[i]).Before(eventTime(sorted[j]))
	})

	omitted := 0
	if len(sorted) > MaxEventsInMessage {
		omitted = len(sorted) - MaxEventsInMessage
		sorted = sorted[len(sorted)-MaxEventsInMessage:]
	}

	var b strings.Builder
	if omitted > 0 {
		fmt.Fprintf(&b, "... %d earlier event(s) omitted\n", omitted)
	}

	for _, ev := range sorted {
		ts := eventTime(ev)
		if ts.IsZero() {
			fmt.Fprintf(&b, "%s %s\n", ev.Reason, ev.Message)
			continue
		}
		// "Aug 25 23:52:21" rather than "2026-08-25 23:52:21.123 +0000 UTC":
		// the year and sub-second precision cost 20 characters per line and
		// tell the reader nothing they need to act.
		fmt.Fprintf(
			&b,
			"%s  %s  %s\n",
			ts.UTC().Format("Jan 02 15:04:05"),
			ev.Reason,
			ev.Message,
		)
	}

	return strings.TrimSpace(b.String())
}

// GetPodContainerLogs returns logs for specified container in pod
func GetPodContainerLogs(
	ctx context.Context,
	c kubernetes.Interface, name, container, namespace string,
	previous bool,
	maxRecentLogLines int64) string {
	options := v1.PodLogOptions{
		Container: container,
		Previous:  previous,
	}

	// get max recent log lines
	if maxRecentLogLines != 0 {
		options.TailLines = &maxRecentLogLines
	} else {
		defaultTail := int64(500)
		options.TailLines = &defaultTail
	}
	limitBytes := int64(1024 * 1024)
	options.LimitBytes = &limitBytes

	// get logs
	logs, err := getContainerLogs(ctx, c, name, namespace, &options)
	if err != nil {
		klog.V(2).InfoS(
			"failed to get logs for container",
			"name", name,
			"container", container,
			"namespace", namespace,
			"error", err.Error())
		return ""
	}

	return string(logs)
}

func getContainerLogs(
	ctx context.Context,
	c kubernetes.Interface,
	name string,
	namespace string,
	options *v1.PodLogOptions) ([]byte, error) {
	// Attempt with 15s timeout; retry once on timeout if context allows.
	for attempt := 0; attempt < 2; attempt++ {
		cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		logs, err := c.CoreV1().Pods(
			namespace,
		).GetLogs(
			name,
			options,
		).DoRaw(
			cctx,
		)
		cancel()

		if err == nil {
			return logs, nil
		}
		if attempt == 0 && cctx.Err() == nil && isTimeoutError(err) {
			klog.V(2).InfoS("log fetch timeout, retrying",
				"container", name, "namespace", namespace)
			continue
		}
		return nil, err
	}
	return nil, fmt.Errorf(
		"log fetch failed after retries for container %s",
		name,
	)
}

func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "context deadline exceeded") ||
		strings.Contains(s, "i/o timeout")
}

// GetPodEvents retrieves the events for a specific pod
func GetPodEvents(
	ctx context.Context,
	c kubernetes.Interface,
	name,
	namespace string) (*v1.EventList, error) {
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return c.CoreV1().Events(namespace).List(cctx, metav1.ListOptions{
		FieldSelector: "involvedObject.name=" + name,
	})
}

// IsNodeReady returns true if the node's Ready condition is True.
func IsNodeReady(n *v1.Node) bool {
	for _, c := range n.Status.Conditions {
		if c.Type == v1.NodeReady {
			return c.Status == v1.ConditionTrue
		}
	}
	return false
}

// GetNodes gets a list of nodes
func GetNodes(
	ctx context.Context,
	c kubernetes.Interface,
) (*v1.NodeList, error) {
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return c.CoreV1().Nodes().List(cctx, metav1.ListOptions{})
}

// GetNodeSummary gets a list of nodes
func GetNodeSummary(
	ctx context.Context,
	c kubernetes.Interface,
	name string,
) ([]byte, error) {
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return c.CoreV1().RESTClient().Get().Resource(
		"nodes",
	).Name(
		name,
	).SubResource(
		"proxy",
	).Suffix(
		"stats/summary",
	).DoRaw(
		cctx,
	)
}

// RandomString generates random string with provided n size
func RandomString(n int) string {
	const availableCharacterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLM" +
		"NOPQRSTUVWXYZ0123456789"

	b := make([]byte, n)
	limit := big.NewInt(int64(len(availableCharacterBytes)))
	for i := range b {
		n, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return ""
		}
		b[i] = availableCharacterBytes[n.Int64()]
	}

	return string(b)
}

// GetNamespace returns the namespace where kwatch is running.
// It reads from POD_NAMESPACE environment variable and falls back to "kwatch".
func GetNamespace() string {
	namespace := os.Getenv("POD_NAMESPACE")
	if namespace == "" {
		return "kwatch"
	}
	return namespace
}
