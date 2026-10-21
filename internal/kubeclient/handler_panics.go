package kubeclient

import (
	"sort"
	"sync"
)

// maxPanicResources bounds the ledger. Resources past it are counted
// together as overflowResource, so a flood of custom kinds cannot grow it.
const (
	maxPanicResources = 128
	overflowResource  = "other"
)

// HandlerPanicCount is the number of recovered handler panics for one
// resource of one component since the process started.
type HandlerPanicCount struct {
	Component string
	Resource  string
	Count     int64
}

// panicLedger remembers recovered handler panics per resource so health
// can show a kind whose callbacks keep failing, which the informer itself
// still reports as synced.
type panicLedger struct {
	mu     sync.Mutex
	counts map[[2]string]int64
}

var handlerPanics = &panicLedger{counts: map[[2]string]int64{}}

// record counts one panic and returns the resource's count so far.
func (l *panicLedger) record(component, resource string) int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	key := [2]string{component, resource}
	if _, known := l.counts[key]; !known &&
		len(l.counts) >= maxPanicResources {
		key = [2]string{component, overflowResource}
	}
	l.counts[key]++
	return l.counts[key]
}

func (l *panicLedger) snapshot() []HandlerPanicCount {
	l.mu.Lock()
	out := make([]HandlerPanicCount, 0, len(l.counts))
	for key, count := range l.counts {
		out = append(out, HandlerPanicCount{
			Component: key[0], Resource: key[1], Count: count,
		})
	}
	l.mu.Unlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Component != out[j].Component {
			return out[i].Component < out[j].Component
		}
		return out[i].Resource < out[j].Resource
	})
	return out
}

// HandlerPanics returns the recovered handler panics seen so far, sorted
// by component and resource. Callers compare successive snapshots to see
// whether a resource is still panicking.
func HandlerPanics() []HandlerPanicCount {
	return handlerPanics.snapshot()
}
