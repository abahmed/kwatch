package detectors

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// recoveredBefore reports whether note is history: the object it is
// about is fully ready now, and it was already ready when the note was
// written or when kwatch first saw it that way. A restarting kwatch
// replays the events of the last minutes; one about a failure the
// object has since recovered from must not open an incident.
// An event that arrives after the object was seen ready still counts.
func recoveredBefore(e inventory.Entity, note inventory.Note) bool {
	since, ok := readySince(e)
	return ok && since.After(note.At)
}

// readySince says whether e is fully ready, and since when: for a
// workload, since its ready count took the value it has; for a pod,
// since it became ready.
func readySince(e inventory.Entity) (time.Time, bool) {
	switch e.ID.Kind {
	case kube.KindDaemonSet:
		desired, ok := number(e, kube.AttrDesiredReplicas)
		ready, _ := number(e, kube.AttrReadyReplicas)
		unavailable, _ := number(e, kube.AttrUnavailable)
		return valueSince(e, kube.AttrReadyReplicas),
			ok && desired > 0 && ready >= desired && unavailable == 0
	case kube.KindDeployment, kube.KindStatefulSet, kube.KindReplicaSet:
		want, ok := number(e, kube.AttrReplicas)
		ready, _ := number(e, kube.AttrReadyReplicas)
		return valueSince(e, kube.AttrReadyReplicas),
			ok && want > 0 && ready >= want
	case kube.KindPod:
		if !flag(e, kube.AttrReady) {
			return time.Time{}, false
		}
		if at := timestamp(e, kube.AttrReadySince); !at.IsZero() {
			return at, true
		}
		return valueSince(e, kube.AttrReady), true
	}
	return time.Time{}, false
}
