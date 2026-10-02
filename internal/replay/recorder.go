package replay

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// Submit is the signature of pipeline.Engine.Submit and of every source's
// submit callback.
type Submit func(context.Context, ...inventory.Observation)

// Recorder writes every observation it forwards as one log line. It stands
// between a source and the engine: pass Recorder.Submit where the engine's
// Submit would go.
type Recorder struct {
	now  func() time.Time
	next Submit

	mu      sync.Mutex
	encoder *json.Encoder
	err     error
}

// NewRecorder writes the log header to w and returns a recorder. now
// stamps each entry with the time the pipeline receives it; next, when
// set, receives the observations after they are written.
func NewRecorder(
	w io.Writer, now func() time.Time, next Submit,
) (*Recorder, error) {
	if w == nil || now == nil {
		return nil, errors.New("replay: recorder needs a writer and a clock")
	}
	encoder := json.NewEncoder(w)
	if err := encoder.Encode(newHeader(now())); err != nil {
		return nil, err
	}
	return &Recorder{now: now, next: next, encoder: encoder}, nil
}

// Submit records the observations, then forwards them. Observations
// submitted together share one timestamp; a replay submits all entries
// with the same timestamp in one call. After the first write error
// nothing more is written; the observations are still forwarded and Err
// reports the failure.
//
// The lock is held while forwarding, so the engine receives submissions
// in the order they were written: a replay of the log then sees what the
// engine saw. A forward that blocks on a full engine queue holds back
// other sources, as the engine itself would.
func (r *Recorder) Submit(
	ctx context.Context, observations ...inventory.Observation,
) {
	r.mu.Lock()
	defer r.mu.Unlock()
	at := r.now().UTC()
	for _, observation := range observations {
		if r.err != nil {
			break
		}
		r.err = r.encoder.Encode(Entry{At: at, Observation: observation})
	}
	if r.next != nil {
		r.next(ctx, observations...)
	}
}

// Err returns the first write error.
func (r *Recorder) Err() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.err
}
