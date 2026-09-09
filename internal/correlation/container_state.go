package correlation

import (
	"time"

	"github.com/abahmed/kwatch/internal/model"
)

// containerStateEntry is the remembered state plus when it was remembered.
//
// The index is pruned when a Pod deletion is observed, and that is the only
// thing that ever pruned it: a delete missed during a restart, an informer
// resync gap, or a namespace kwatch stopped watching left the entry in memory
// for the life of the process. On a cluster running CronJobs that is a slow,
// permanent leak, one entry per container that ever restarted. The timestamp
// gives the sweep something to expire.
type containerStateEntry struct {
	state     *model.ContainerState
	indexedAt time.Time
}

// containerStateTTL is how long a container's last-known state is kept after
// it was last written. It only feeds "what happened the previous time this
// container died", which stops being interesting long before a day is out.
const containerStateTTL = 24 * time.Hour

// pruneContainerStates drops container state nobody asked about for a day.
// Caller must hold e.mu.
func (e *Engine) pruneContainerStates(now time.Time) {
	for key, entry := range e.lastContainerIndex {
		if now.Sub(entry.indexedAt) > containerStateTTL {
			delete(e.lastContainerIndex, key)
		}
	}
}

func lastContainerKey(namespace, podName, container string) string {
	if container == "" || container == "." {
		container = "."
	}
	return namespace + "/" + podName + "/" + container
}

// Caller must hold e.mu.
func (e *Engine) indexLastContainerState(
	namespace, podName, container string,
	cs *model.ContainerState,
) {
	if podName == "" || cs == nil {
		return
	}
	cp := *cs
	e.lastContainerIndex[lastContainerKey(namespace, podName, container)] =
		containerStateEntry{state: &cp, indexedAt: e.now()}
}
