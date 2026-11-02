package dashboard

import (
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
)

// Live update bounds: open streams at once, and from one client (a
// session, or an address for the bearer token), how often the incidents
// are compared while a stream is open, the heartbeat, the IDs one
// event names, the changes remembered for a stream that is behind,
// and the heartbeats before a stream ends (the browser then
// reconnects, and its sign-in is checked again).
const (
	maxLiveConns  = 50
	maxClientLive = 3
	liveCheck     = 2 * time.Second
	liveHeartbeat = 25 * time.Second
	maxLiveIDs    = 50
	liveRingSize  = 32
	liveMaxBeats  = 72
	liveWriteWait = 10 * time.Second
	// liveRetryMillis is how long the browser waits before it
	// reconnects a dropped stream.
	liveRetryMillis = 5000
)

// liveTicker is a ticker the hub can fake in tests.
type liveTicker struct {
	C    <-chan time.Time
	stop func()
}

func realTicker(d time.Duration) liveTicker {
	t := time.NewTicker(d)
	return liveTicker{C: t.C, stop: t.Stop}
}

// liveChange is one comparison that found changed incidents.
type liveChange struct {
	seq uint64
	ids []string
}

// liveHub tells open /api/events streams that incidents changed. It
// has no goroutine of its own: the streams' tickers drive check, which
// compares the incidents at most once per liveCheck and only after the
// engine ran again. Its state is dropped when the last stream closes.
type liveHub struct {
	mu    sync.Mutex
	conns int
	// byClient counts the open streams of each client.
	byClient  map[string]int
	revs      map[string]string
	version   string
	seq       uint64
	ring      []liveChange
	checkedAt time.Time
	scanned   int64
	wake      chan struct{}
	done      chan struct{}
	closed    bool
	tick      func(time.Duration) liveTicker
	// wall is the real time write deadlines are set from.
	wall func() time.Time
}

func newLiveHub() *liveHub {
	return &liveHub{wake: make(chan struct{}), done: make(chan struct{}),
		tick: realTicker, wall: time.Now}
}

// Why acquire refused a stream slot.
const (
	liveOK = iota
	liveFull
	liveClientFull
)

// acquire takes a stream slot for client: liveFull when all are taken
// or the server is shutting down, liveClientFull when client already
// has maxClientLive streams open.
func (h *liveHub) acquire(client string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed || h.conns >= maxLiveConns {
		return liveFull
	}
	if h.byClient[client] >= maxClientLive {
		return liveClientFull
	}
	if h.byClient == nil {
		h.byClient = map[string]int{}
	}
	h.byClient[client]++
	h.conns++
	return liveOK
}

// release gives client's slot back; the last one forgets the
// incidents.
func (h *liveHub) release(client string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.conns--
	if h.byClient[client]--; h.byClient[client] <= 0 {
		delete(h.byClient, client)
	}
	if h.conns == 0 {
		h.revs, h.ring, h.version = nil, nil, ""
		h.checkedAt, h.scanned = time.Time{}, 0
	}
}

// close ends every stream: the server is shutting down.
func (h *liveHub) close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.closed {
		h.closed = true
		close(h.done)
	}
}

// liveState is where a stream stands: the last change it has seen,
// the list's version, and the channel closed at the next change.
type liveState struct {
	seq     uint64
	version string
	wake    <-chan struct{}
}

func (h *liveHub) state() liveState {
	h.mu.Lock()
	defer h.mu.Unlock()
	return liveState{seq: h.seq, version: h.version, wake: h.wake}
}

// check compares the incidents with the last comparison when the
// engine ran since (scan is its latest loop time) and liveCheck has
// passed, and wakes the streams when any changed. red redacts the
// texts that are fingerprinted.
func (h *liveHub) check(
	now time.Time, scan int64, read func() []incident.Incident,
	red func(string) string,
) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.revs != nil &&
		(scan == h.scanned || now.Sub(h.checkedAt) < liveCheck) {
		return
	}
	h.checkedAt, h.scanned = now, scan
	revs := liveRevs(read(), red)
	first := h.revs == nil
	ids := changedIDs(h.revs, revs)
	h.revs = revs
	h.version = liveVersion(revs)
	if first || len(ids) == 0 {
		return
	}
	h.seq++
	h.ring = append(h.ring, liveChange{seq: h.seq,
		ids: ids[:min(len(ids), maxLiveIDs+1)]})
	if over := len(h.ring) - liveRingSize; over > 0 {
		h.ring = slices.Clone(h.ring[over:])
	}
	close(h.wake)
	h.wake = make(chan struct{})
}

// since is the IDs changed after seq, at most maxLiveIDs; more is set
// when there were more, or when changes older than the ring were
// missed, so the stream should read everything again.
func (h *liveHub) since(seq uint64) (ids []string, more bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	seen := map[string]bool{}
	for i, c := range h.ring {
		if c.seq <= seq {
			continue
		}
		if i == 0 && c.seq > seq+1 {
			more = true
		}
		for _, id := range c.ids {
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	if len(h.ring) == 0 && h.seq > seq {
		more = true
	}
	slices.Sort(ids)
	if len(ids) > maxLiveIDs {
		return ids[:maxLiveIDs], true
	}
	return ids, more
}

// liveRevs fingerprints each incident: what its row shows, plus what
// its page shows that moves (messages sent, timeline, members). The row
// is fingerprinted redacted, as the API shows it, so the version a
// stream sends says nothing about a redacted secret.
func liveRevs(
	items []incident.Incident, red func(string) string,
) map[string]string {
	out := make(map[string]string, len(items))
	for _, p := range items {
		row := itemRev(toListItemWith(p, red))
		out[p.ID] = shortHash(row + "\x00" + strconv.Itoa(p.Revision) +
			"\x00" + strconv.Itoa(len(p.Timeline)) + "\x00" +
			strconv.Itoa(len(p.Members)))
	}
	return out
}

// changedIDs are the IDs added, removed or changed between old and
// now, sorted.
func changedIDs(old, now map[string]string) []string {
	var out []string
	for id, rev := range now {
		if old[id] != rev {
			out = append(out, id)
		}
	}
	for id := range old {
		if _, ok := now[id]; !ok {
			out = append(out, id)
		}
	}
	slices.Sort(out)
	return out
}

// liveVersion fingerprints the whole set, like an ETag of it.
func liveVersion(revs map[string]string) string {
	ids := make([]string, 0, len(revs))
	for id := range revs {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	var b strings.Builder
	for _, id := range ids {
		b.WriteString(id)
		b.WriteByte(0)
		b.WriteString(revs[id])
		b.WriteByte(0)
	}
	return shortHash(b.String())
}
