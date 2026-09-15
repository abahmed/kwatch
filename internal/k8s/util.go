package k8s

import (
	"context"
	"crypto/rand"
	"math/big"
	"os"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/kubelet"
)

// MaxEventsInMessage remains the compatibility name for callers of the
// original Kubernetes helper. Event formatting is owned by internal/event.
const MaxEventsInMessage = event.MaxPodEventsInMessage

func GetPodEventsStr(events *[]v1.Event) string {
	return event.FormatPodEvents(events)
}

// GetPodContainerLogs is kept as a compatibility wrapper for callers of the
// original Kubernetes helper package.
func GetPodContainerLogs(
	ctx context.Context,
	c kubernetes.Interface, name, container, namespace string,
	previous bool,
	maxRecentLogLines int64) string {
	return kubelet.GetPodContainerLogs(
		ctx,
		c,
		name,
		container,
		namespace,
		previous,
		maxRecentLogLines,
	)
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
