package pipeline

import (
	"context"
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/metrics"
)

// maxPending bounds observations waiting for the pipeline. Informers deliver in
// bursts during relists; beyond this, Submit blocks the caller briefly
// rather than dropping state.
const maxPending = 50000

// inbox is the queue between the sources and the decision loop. Sources
// add observations from any goroutine; the loop takes them all at once.
// Every field except lag.drained is guarded by mu.
type inbox struct {
	mu      sync.Mutex
	pending []inventory.Observation
	// wake tells the loop that observations are waiting.
	wake chan struct{}
	// space tells a blocked Submit that the loop emptied the queue.
	space chan struct{}
	lag   decisionLag
}

func newInbox() *inbox {
	return &inbox{
		wake:  make(chan struct{}, 1),
		space: make(chan struct{}, 1),
	}
}

// decisionLag times observations from Submit until the loop iteration
// that applied them has made its decisions. Only the oldest pending
// observation is timed: it waited longest, so it bounds the batch.
type decisionLag struct {
	// pending is when the oldest observation still in the queue was
	// submitted; zero when the queue is empty. Guarded by inbox.mu.
	pending time.Time
	// drained is the submit time of the oldest observation the loop has
	// taken but not yet decided on. Read and written by the loop only.
	drained time.Time
}

// Submit queues observations for the pipeline. It blocks only while the
// queue is full, and returns when ctx ends.
func (e *Engine) Submit(
	ctx context.Context, observations ...inventory.Observation,
) {
	submitted := e.deps.Clock.Now()
	in := e.inbox
	for {
		in.mu.Lock()
		if len(in.pending)+len(observations) <= maxPending ||
			len(in.pending) == 0 {
			if len(in.pending) == 0 && len(observations) > 0 {
				in.lag.pending = submitted
			}
			in.pending = append(in.pending, observations...)
			in.mu.Unlock()
			notify(in.wake)
			return
		}
		in.mu.Unlock()
		select {
		case <-in.space:
		case <-ctx.Done():
			return
		}
	}
}

// drain applies pending observations to the model and returns the
// touched entities, each once.
func (e *Engine) drain() []inventory.EntityID {
	return e.apply(e.inbox.take())
}

// take empties the queue and returns what was in it.
func (in *inbox) take() []inventory.Observation {
	in.mu.Lock()
	observations := in.pending
	in.pending = nil
	if in.lag.drained.IsZero() {
		in.lag.drained = in.lag.pending
	}
	in.lag.pending = time.Time{}
	in.mu.Unlock()
	notify(in.space)
	return observations
}

// observeLag records the decision lag of the observations drained since
// the last call. It runs at the end of a loop iteration, once their
// findings were evaluated and the incident decisions applied.
func (e *Engine) observeLag() {
	lag := &e.inbox.lag
	since := lag.drained
	if since.IsZero() {
		return
	}
	lag.drained = time.Time{}
	waited := e.deps.Clock.Now().Sub(since)
	metrics.DefaultRegistry().ObserveDecisionLag(waited.Seconds())
}
