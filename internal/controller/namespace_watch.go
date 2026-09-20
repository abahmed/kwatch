package controller

import (
	"context"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

// namespaceRecheckInterval is how often a label-selected namespace scope is
// re-evaluated.
const namespaceRecheckInterval = time.Minute

// WatchNamespaceScope re-evaluates a namespaceSelector scope and asks for a
// restart when the set of matching namespaces changes.
//
// The selector was resolved exactly once, during New. A namespace created
// afterwards that matched the selector was never watched -- no informer, no
// detectors, no alerts for anything in it -- and nothing said so; the only
// cure was a restart somebody had to think of. Re-resolving and restarting is
// the same mechanism the CRD watcher already uses for a configuration change,
// and a brief restart is easier to reason about than re-wiring informer
// factories underneath a running controller.
//
// It does nothing when the scope is static (explicit namespaces or
// cluster-wide), because then there is nothing to re-resolve.
func WatchNamespaceScope(
	ctx context.Context,
	client kubernetes.Interface,
	selector string,
	known []string,
	restart func(),
) {
	if selector == "" || restart == nil {
		return
	}
	want := append([]string(nil), known...)
	sort.Strings(want)
	ticker := time.NewTicker(namespaceRecheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			current, err := selectedNamespaces(ctx, client, selector)
			if err != nil {
				klog.V(2).InfoS(
					"namespace selector re-check failed",
					"error", err,
				)
				continue
			}
			if sameNamespaces(want, current) {
				continue
			}
			klog.InfoS(
				"namespaceSelector scope changed; restarting to watch the "+
					"new set",
				"before", want, "after", current,
			)
			restart()
			return
		}
	}
}

func selectedNamespaces(
	ctx context.Context,
	client kubernetes.Interface,
	selector string,
) ([]string, error) {
	listCtx, cancel := context.WithTimeout(ctx, namespaceResolveTimeout)
	defer cancel()
	list, err := client.CoreV1().Namespaces().List(listCtx, metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(list.Items))
	for _, item := range list.Items {
		out = append(out, item.Name)
	}
	sort.Strings(out)
	return out, nil
}

func sameNamespaces(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
