package kube

import (
	"context"
	"errors"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes"

	"github.com/abahmed/kwatch/internal/inventory"
)

// Evidence read bounds. A report log tail is longer than the message
// excerpt: the dashboard shows it whole.
const (
	reportTailLines = 500
	reportKeepBytes = 128 << 10
	readyzBudget    = 2 * time.Second
	maxReadyzLines  = 60
)

// IsDenied reports an API read refused for lack of permission.
func IsDenied(err error) bool {
	return apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err)
}

// IsAbsent reports a read of something that is not there: a previous
// run of a container that never restarted, or a missing object.
func IsAbsent(err error) bool {
	return apierrors.IsNotFound(err) || apierrors.IsBadRequest(err)
}

// Tail returns one run of a container's output for the evidence report:
// the previous run when previous is set, else the current one, oldest
// first, redacted and bounded to reportTailLines lines. The error says
// why a read failed (see IsDenied and IsAbsent).
func (r LogReader) Tail(
	ctx context.Context, container inventory.EntityID, previous bool,
) ([]string, error) {
	body, err := r.read(ctx, container, previous, reportTailLines)
	if err != nil {
		return nil, err
	}
	return logLines(lastBytes(body, reportKeepBytes)), nil
}

// ReadyzReader reads the API server's own readiness checks.
type ReadyzReader struct {
	Client kubernetes.Interface
}

// Checks returns the lines of /readyz?verbose: "[+]etcd ok" for each
// check, bounded to maxReadyzLines.
func (r ReadyzReader) Checks(ctx context.Context) ([]string, error) {
	if r.Client == nil {
		return nil, errors.New("no client")
	}
	readCtx, cancel := context.WithTimeout(ctx, readyzBudget)
	defer cancel()
	body, err := r.Client.Discovery().RESTClient().Get().
		AbsPath("/readyz").Param("verbose", "").DoRaw(readCtx)
	// A failing check answers 500 with the same body, which is the
	// evidence: keep it.
	if err != nil && len(body) == 0 {
		return nil, err
	}
	var out []string
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && len(out) < maxReadyzLines {
			out = append(out, truncate(line))
		}
	}
	return out, nil
}
