package delivery

import (
	"slices"
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/delivery/transport"
)

// SendAttempt is what became of one message of an incident at one
// provider: the dashboard shows these as who was told. Provider is
// empty when no provider received the message (a digest or summary
// carried it, or nothing was routed). Error is scrubbed like the send
// log line. It is persisted in TrailRecord.
type SendAttempt struct {
	At        time.Time `json:"at"`
	Provider  string    `json:"provider,omitempty"`
	Kind      string    `json:"kind,omitempty"`
	Placement string    `json:"placement,omitempty"`
	// Result is ok, failed (given up or handed to the fallback),
	// retrying (sent again later), carried, not routed or paging only.
	Result   string `json:"result"`
	Retries  int    `json:"retries,omitempty"`
	Carrier  string `json:"carrier,omitempty"`
	Fallback string `json:"fallback,omitempty"`
	Error    string `json:"error,omitempty"`
}

// Send trail bounds: the incidents remembered, the least recently told
// forgotten first, and the attempts kept for each: the newest, plus the
// first accepted one. They bound the saved trail too.
const (
	maxTrailKeys     = 512
	maxTrailAttempts = 16
)

// sendTrail keeps the latest attempts per incident. Once the outbox is
// attached it is also saved (see send_trail_store.go): dirty names the
// keys changed (true) or forgotten (false) since the last write, and
// changed wakes the writer. The zero value is ready to use.
type sendTrail struct {
	mu      sync.Mutex
	byKey   map[string][]SendAttempt
	order   []string
	dirty   map[string]bool
	changed func()
}

func (t *sendTrail) add(key string, a SendAttempt) {
	t.mu.Lock()
	if t.byKey == nil {
		t.byKey = map[string][]SendAttempt{}
	}
	list, seen := t.byKey[key]
	if seen {
		// The order is by last use: an incident still being told
		// is never forgotten before one that went quiet.
		t.order = slices.DeleteFunc(t.order, func(k string) bool {
			return k == key
		})
	}
	t.order = append(t.order, key)
	t.byKey[key] = keepNewest(append(list, a))
	t.markLocked(key, true)
	t.boundLocked()
	changed := t.changed
	t.mu.Unlock()
	if changed != nil {
		changed()
	}
}

// keepNewest is list without the attempts past the bound, oldest first.
// The first accepted attempt is kept in front of the newest: it is when
// people were first told, which the dashboard shows.
func keepNewest(list []SendAttempt) []SendAttempt {
	over := len(list) - maxTrailAttempts
	if over <= 0 {
		return list
	}
	first := slices.IndexFunc(list, func(a SendAttempt) bool {
		return a.Result == resultOK
	})
	if first < 0 || first >= over {
		return append([]SendAttempt(nil), list[over:]...)
	}
	out := make([]SendAttempt, 0, maxTrailAttempts)
	out = append(out, list[first])
	return append(out, list[over+1:]...)
}

// boundLocked forgets the oldest incidents past the bound. The caller
// holds t.mu.
func (t *sendTrail) boundLocked() {
	for len(t.order) > maxTrailKeys {
		delete(t.byKey, t.order[0])
		t.markLocked(t.order[0], false)
		t.order = t.order[1:]
	}
}

// markLocked notes that key must be written (live) or deleted. The
// caller holds t.mu.
func (t *sendTrail) markLocked(key string, live bool) {
	if t.dirty == nil {
		t.dirty = map[string]bool{}
	}
	t.dirty[key] = live
}

func (t *sendTrail) of(key string) []SendAttempt {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]SendAttempt(nil), t.byKey[key]...)
}

// SendTrail returns the remembered delivery attempts of the incident
// with id key, oldest first; nil when none are remembered.
func (m *Manager) SendTrail(key string) []SendAttempt {
	return m.trail.of(key)
}

// noteSent remembers one provider send of an incident message.
func (m *Manager) noteSent(r sendRecord, job deliverJob) {
	if !trailed(job) {
		return
	}
	a := SendAttempt{At: m.nowTime(), Provider: r.provider,
		Kind: r.kind, Placement: r.placement, Result: sendResult(r.err),
		Retries: r.retries, Fallback: r.fallback}
	if r.err != nil {
		a.Error = loggedError(r.err)
	}
	m.trail.add(job.key(), a)
}

// resultOK is the result of an attempt a provider accepted.
const resultOK = "ok"

// sendResult is the trail's word for how a send ended: a permanent
// error is given up (or handed to the fallback), any other is retried.
func sendResult(err error) string {
	switch {
	case err == nil:
		return resultOK
	case transport.IsPermanent(err):
		return "failed"
	}
	return "retrying"
}

// noteUnsent writes the send log line of a message no provider
// received and remembers it in the trail.
func (m *Manager) noteUnsent(job deliverJob, result, carrier string) {
	logUnsent(job, result, carrier)
	m.trailUnsent(job, result, carrier)
}

// trailUnsent remembers a message no provider received in the trail.
func (m *Manager) trailUnsent(job deliverJob, result, carrier string) {
	if !trailed(job) {
		return
	}
	kind, placement := describeJob(job)
	m.trail.add(job.key(), SendAttempt{At: m.nowTime(), Kind: kind,
		Placement: placement, Result: result, Carrier: carrier})
}

// trailed reports a job the trail remembers: a message of one incident.
// Summaries, roll-ups and digests are not incidents: the dashboard never
// asks for their keys, so they would only take an incident's place.
func trailed(job deliverJob) bool {
	return job.kind == jobIncident && job.incident != nil &&
		!job.incident.IsSummary()
}
