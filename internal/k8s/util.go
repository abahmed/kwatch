package k8s

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

// GetPodEventsStr returns formatted events as a string for specified pod
func GetPodEventsStr(events *[]v1.Event) string {
	if events == nil {
		return ""
	}

	eventsString := ""

	for _, ev := range *events {
		ts := ev.LastTimestamp.Time
		if ts.IsZero() {
			ts = ev.EventTime.Time
		}
		if ts.IsZero() {
			ts = ev.FirstTimestamp.Time
		}
		if ts.IsZero() {
			ts = ev.CreationTimestamp.Time
		}

		if ts.IsZero() {
			eventsString += fmt.Sprintf("%s %s\n", ev.Reason, ev.Message)
			continue
		}

		eventsString +=
			fmt.Sprintf(
				"[%s] %s %s\n",
				ts.String(),
				ev.Reason,
				ev.Message)
	}

	return strings.TrimSpace(eventsString)
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
		logs, err := c.CoreV1().
			Pods(namespace).
			GetLogs(name, options).
			DoRaw(cctx)
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
	return nil, fmt.Errorf("log fetch failed after retries for container %s", name)
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
	return c.CoreV1().
		Events(namespace).
		List(cctx, metav1.ListOptions{
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
func GetNodes(ctx context.Context, c kubernetes.Interface) (*v1.NodeList, error) {
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return c.CoreV1().
		Nodes().
		List(cctx, metav1.ListOptions{})
}

// GetNodeSummary gets a list of nodes
func GetNodeSummary(ctx context.Context, c kubernetes.Interface, name string) ([]byte, error) {
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return c.CoreV1().
		RESTClient().
		Get().
		Resource("nodes").
		Name(name).
		SubResource("proxy").
		Suffix("stats/summary").
		DoRaw(cctx)
}

// RandomString generates random string with provided n size
func RandomString(n int) string {
	const availableCharacterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLM" +
		"NOPQRSTUVWXYZ0123456789"

	b := make([]byte, n)
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := range b {
		b[i] = availableCharacterBytes[r.Intn(len(availableCharacterBytes))]
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
