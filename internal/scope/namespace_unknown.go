package scope

import (
	"sort"
	"sync"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
)

// NamespaceHold is how long the namespace label selector waits for a
// Namespace it cannot see before it lets the namespace in. A Namespace
// that is only late (created a moment ago, or listed after its pods)
// shows up well inside it. One that never shows up cannot be read: the
// selector then cannot be checked at all, and saying nothing about a
// real failure is worse than saying it about an excluded namespace.
const NamespaceHold = 30 * time.Second

// maxUnknownNamespaces bounds the namespaces remembered as not seen.
const maxUnknownNamespaces = 1024

// unknownNamespaces remembers the namespaces the selector was asked
// about while their Namespace was not in the model: since when, and
// whether the wait ran out and the namespace was let in unchecked.
type unknownNamespaces struct {
	mu    sync.Mutex
	since map[string]time.Time
	open  map[string]bool
}

func newUnknownNamespaces() *unknownNamespaces {
	return &unknownNamespaces{
		since: map[string]time.Time{}, open: map[string]bool{},
	}
}

// seen forgets a namespace whose Namespace is in the model again.
func (u *unknownNamespaces) seen(namespace string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	delete(u.since, namespace)
	delete(u.open, namespace)
}

// allow decides for a namespace whose Namespace is not in the model:
// out while it is held, in once NamespaceHold has passed. Without a
// clock the wait cannot end, so the namespace stays held. With note it
// starts the wait and logs each step once; without, it only reads.
func (u *unknownNamespaces) allow(
	namespace string, now func() time.Time, note bool,
) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.open[namespace] {
		return true
	}
	if now == nil {
		return false
	}
	at := now()
	first, held := u.since[namespace]
	if !held {
		if len(u.since) >= maxUnknownNamespaces {
			// Too many to time one by one: let it in, unremembered.
			return true
		}
		if !note {
			return false
		}
		u.since[namespace] = at
		klog.InfoS("namespace selector: the Namespace is not known yet; "+
			"holding the decision", "component", "scope",
			"operation", "namespace-selector", "namespace", namespace,
			"hold", NamespaceHold.String())
		return false
	}
	if at.Sub(first) < NamespaceHold {
		return false
	}
	if note {
		u.open[namespace] = true
		klog.InfoS("WARNING: namespace selector cannot be checked: the "+
			"Namespace cannot be read, so its problems are reported as "+
			"in scope", "component", "scope",
			"operation", "namespace-selector", "namespace", namespace)
	}
	return true
}

// unchecked lists the namespaces let in without a look at their labels.
func (u *unknownNamespaces) unchecked() []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	out := make([]string, 0, len(u.open))
	for namespace := range u.open {
		out = append(out, namespace)
	}
	sort.Strings(out)
	return out
}

// Pending reports that Allows refuses the finding only for now: its
// namespace passes the namespace lists, but the label selector waits
// for a Namespace that is not in the model yet (see NamespaceHold). The
// refusal is not a verdict: ask again on a later tick. It never starts
// or ends a wait itself. A finding another rule refuses (its reason, a
// maintenance window, a silence) is refused for good, not pending.
func (s *Scope) Pending(model inventory.Reader, sig detection.Finding) bool {
	return s != nil && s.NamespacePending(model, sig.Entity.Namespace) &&
		s.findingAllowed(model, sig)
}

// NamespacePending is Pending for a namespace.
func (s *Scope) NamespacePending(
	model inventory.Reader, namespace string,
) bool {
	if s == nil || s.selector == nil || namespace == "" ||
		!s.listed(namespace) {
		return false
	}
	if _, known := namespaceLabels(model, namespace); known {
		return false
	}
	return !s.unknown.allow(namespace, s.now, false)
}

// UncheckedNamespaces lists, sorted, the namespaces the label selector
// let in without seeing their labels, because their Namespace could not
// be read within NamespaceHold. It is empty without a selector and
// while every Namespace asked about is readable: health and coverage
// can show it as a gap.
func (s *Scope) UncheckedNamespaces() []string {
	if s == nil || s.unknown == nil {
		return nil
	}
	return s.unknown.unchecked()
}
