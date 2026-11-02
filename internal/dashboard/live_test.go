package dashboard

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/incident"
)

// liveRig is a server whose clock and tickers the test moves.
type liveRig struct {
	s     *Server
	items *fakeIncidents
	mu    sync.Mutex
	now   time.Time
	poll  chan time.Time
	beat  chan time.Time
}

func newLiveRig(t *testing.T, items ...incident.Incident) *liveRig {
	t.Helper()
	g := &liveRig{now: testEpoch, poll: make(chan time.Time),
		beat: make(chan time.Time), items: &fakeIncidents{items: items}}
	g.s = NewServer(Options{Token: testToken},
		clock.Func(func() time.Time {
			g.mu.Lock()
			defer g.mu.Unlock()
			return g.now
		}))
	g.s.SetIncidents(g.items)
	g.s.live.tick = func(d time.Duration) liveTicker {
		if d == liveHeartbeat {
			return liveTicker{C: g.beat, stop: func() {}}
		}
		return liveTicker{C: g.poll, stop: func() {}}
	}
	g.s.NoteScan()
	return g
}

// change replaces the incidents, moves the clock past liveCheck and
// records an engine loop.
func (g *liveRig) change(items ...incident.Incident) {
	g.items.mu.Lock()
	g.items.items = items
	g.items.mu.Unlock()
	g.mu.Lock()
	g.now = g.now.Add(liveCheck)
	g.mu.Unlock()
	g.s.NoteScan()
}

// liveClient reads one stream's events.
type liveClient struct {
	cancel context.CancelFunc
	resp   *http.Response
	lines  *bufio.Scanner
}

func openLive(t *testing.T, url string) *liveClient {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		url+"/api/events", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+testToken)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { cancel(); _ = resp.Body.Close() })
	return &liveClient{cancel: cancel, resp: resp,
		lines: bufio.NewScanner(resp.Body)}
}

// next is the next event's name and data.
func (c *liveClient) next(t *testing.T) (string, LiveEvent) {
	t.Helper()
	var name string
	var data LiveEvent
	for c.lines.Scan() {
		line := c.lines.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			name = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			require.NoError(t, json.Unmarshal(
				[]byte(strings.TrimPrefix(line, "data: ")), &data))
		case line == "" && name != "":
			return name, data
		}
	}
	return "", data
}

func TestLiveStreamsChangesAndHeartbeats(t *testing.T) {
	a := testIncident("a", "shop", -time.Hour)
	b := testIncident("b", "shop", -time.Hour)
	g := newLiveRig(t, a, b)
	srv := httptest.NewServer(g.s.Handler())
	t.Cleanup(srv.Close)

	c := openLive(t, srv.URL)
	assert.Equal(t, "text/event-stream",
		c.resp.Header.Get("Content-Type"))
	assert.Equal(t, "no-store", c.resp.Header.Get("Cache-Control"))
	assert.Equal(t, "no", c.resp.Header.Get("X-Accel-Buffering"))
	name, ready := c.next(t)
	require.Equal(t, "ready", name)
	require.NotEmpty(t, ready.Version)

	b.Tier = incident.Page
	g.change(a, b, testIncident("c", "web", 0))
	g.poll <- testEpoch
	name, ev := c.next(t)
	require.Equal(t, "changed", name)
	assert.Equal(t, []string{"b", "c"}, ev.IDs)
	assert.False(t, ev.More)
	assert.NotEqual(t, ready.Version, ev.Version)

	g.beat <- testEpoch
	name, _ = c.next(t)
	assert.Equal(t, "ping", name)
}

func TestLiveSkipsAComparisonUntilTheEngineRan(t *testing.T) {
	g := newLiveRig(t, testIncident("a", "shop", 0))
	g.s.checkLive()
	before := g.s.live.state()

	g.items.mu.Lock()
	g.items.items = nil
	g.items.mu.Unlock()
	g.mu.Lock()
	g.now = g.now.Add(time.Minute)
	g.mu.Unlock()
	g.s.checkLive()
	assert.Equal(t, before.seq, g.s.live.state().seq)

	g.s.NoteScan()
	g.s.checkLive()
	assert.Equal(t, before.seq+1, g.s.live.state().seq)
}

func TestLiveSinceIsBoundedAndSaysWhenItMissedChanges(t *testing.T) {
	h := newLiveHub()
	h.revs = map[string]string{}
	for i := range liveRingSize + 3 {
		h.seq++
		h.ring = append(h.ring, liveChange{seq: h.seq,
			ids: []string{"id-" + strconv.Itoa(i)}})
	}
	h.ring = h.ring[3:]

	ids, more := h.since(h.seq - 1)
	assert.Equal(t, []string{"id-34"}, ids)
	assert.False(t, more)
	ids, more = h.since(0)
	assert.Len(t, ids, liveRingSize)
	assert.True(t, more)

	big := map[string]string{}
	for i := range maxLiveIDs * 2 {
		big["x"+strconv.Itoa(i)] = "1"
	}
	h.checkedAt = time.Time{}
	h.check(testEpoch, 1, func() []incident.Incident {
		out := []incident.Incident{}
		for id := range big {
			out = append(out, incident.Incident{ID: id})
		}
		return out
	}, redactNone)
	ids, more = h.since(h.seq - 1)
	assert.Len(t, ids, maxLiveIDs)
	assert.True(t, more)
}

func TestLiveRefusesStreamsPastTheLimit(t *testing.T) {
	g := newLiveRig(t)
	g.s.live.conns = maxLiveConns

	w := get(t, g.s, "/api/events")

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Equal(t, "30", w.Header().Get("Retry-After"))
}

func TestLiveNeedsAuthGetAndAPipeline(t *testing.T) {
	g := newLiveRig(t)
	assert.Equal(t, http.StatusUnauthorized,
		call(t, g.s, http.MethodGet, "/api/events", "").Code)
	assert.Equal(t, http.StatusMethodNotAllowed,
		call(t, g.s, http.MethodPost, "/api/events", testToken).Code)
	g.s.SetIncidents(nil)
	assert.Equal(t, http.StatusServiceUnavailable,
		get(t, g.s, "/api/events").Code)
}

func TestLiveReleasesTheSlotWhenTheClientLeaves(t *testing.T) {
	g := newLiveRig(t, testIncident("a", "shop", 0))
	srv := httptest.NewServer(g.s.Handler())
	t.Cleanup(srv.Close)
	c := openLive(t, srv.URL)
	name, _ := c.next(t)
	require.Equal(t, "ready", name)

	c.cancel()

	require.Eventually(t, func() bool {
		g.s.live.mu.Lock()
		defer g.s.live.mu.Unlock()
		return g.s.live.conns == 0 && g.s.live.revs == nil
	}, 5*time.Second, time.Millisecond)
}

func TestLiveEndsStreamsAtShutdownAndAfterTheLastBeat(t *testing.T) {
	g := newLiveRig(t)
	srv := httptest.NewServer(g.s.Handler())
	t.Cleanup(srv.Close)

	c := openLive(t, srv.URL)
	name, _ := c.next(t)
	require.Equal(t, "ready", name)
	for range liveMaxBeats - 1 {
		g.beat <- testEpoch
		name, _ = c.next(t)
		require.Equal(t, "ping", name)
	}
	g.beat <- testEpoch
	name, _ = c.next(t)
	assert.Empty(t, name, "the stream ends after the last beat")

	c = openLive(t, srv.URL)
	name, _ = c.next(t)
	require.Equal(t, "ready", name)
	g.s.live.close()
	name, _ = c.next(t)
	assert.Empty(t, name, "the stream ends at shutdown")
	assert.Equal(t, liveFull, g.s.live.acquire("someone"))
}
