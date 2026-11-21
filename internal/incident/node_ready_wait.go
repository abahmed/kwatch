package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// notReadyWait is how long a node's Ready condition must stay false before
// the node detector reports NotReady (detectors.DefaultNodeNotReady), plus
// a second so the finding is in by the time the wait ends.
const notReadyWait = 90*time.Second + time.Second

// waitForNotReady lengthens settle for an incident rooted at a node whose
// Ready condition went false but has not yet been reported as NotReady.
// A kubelet event about such a node (the runtime cannot garbage-collect
// containers, the kubelet is unhealthy) is a symptom of the NotReady that
// is about to be reported; announcing the symptom first costs a second
// message when the node page follows. The wait ends when the finding
// arrives or notReadyWait after the condition changed.
func (m *Manager) waitForNotReady(
	p *Incident, settle time.Duration,
) time.Duration {
	if m.model == nil || p.Root.Kind != kube.KindNode {
		return settle
	}
	for _, f := range p.Members {
		if f.Reason == reasons.NodeNotReady ||
			f.Reason == reasons.NodeHeartbeatStale {
			return settle
		}
	}
	ready, ok := m.model.Attribute(p.Root, kube.AttrReady)
	if !ok || ready.Since.IsZero() {
		return settle
	}
	if v, isBool := ready.Value.AsBool(); !isBool || v {
		return settle
	}
	return max(settle, ready.Since.Add(notReadyWait).Sub(p.Opened))
}
