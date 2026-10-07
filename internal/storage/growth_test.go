package storage

import (
	"context"
	"fmt"
	"math/rand"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The churn below imitates a busy staging cluster: a few hundred open
// and recently resolved incidents that reopen and change, a few hundred
// fingerprints, and a steady stream of timeline entries (changes,
// events, findings, decisions). Counts are scaled down so the test is
// fast; the shape is what matters: what must stay is bounded by the
// retention windows, so the file must be too.
const (
	// churnStep is the simulated time between compactor passes; the
	// counts below are per hour and are scaled by it.
	churnStep              = 6
	churnIncidentBytes     = 3000
	churnTimelinePerHour   = 200
	churnTimelineEntryText = 220
	churnEntities          = 400
	churnFingerprints      = 404
)

// churn drives one store through simulated hours.
type churn struct {
	t         *testing.T
	store     *Store
	clock     *fakeClock
	rng       *rand.Rand
	incidents *Mirror[string]
	prints    *Mirror[string]
	live      map[string]Item[string]
	prints404 map[string]Item[string]
	next      int
}

func newChurn(t *testing.T, s *Store, clock *fakeClock) *churn {
	c := &churn{
		t: t, store: s, clock: clock, rng: rand.New(rand.NewSource(7)),
		incidents: NewMirror(IncidentRecords[string](s)),
		prints:    NewMirror(FingerprintValues[string](s)),
		live:      map[string]Item[string]{},
		prints404: map[string]Item[string]{},
	}
	for i := 0; i < churnFingerprints; i++ {
		c.prints404[fmt.Sprintf("pod/ns/p-%d", i)] = Item[string]{Value: "d0"}
	}
	for i := 0; i < 300; i++ {
		c.open()
	}
	return c
}

func (c *churn) record(rev int) string {
	text := strings.Repeat(string(rune('a'+rev%26)), churnIncidentBytes)
	return fmt.Sprintf(`{"rev":%d,"text":"%s"}`, rev, text)
}

func (c *churn) open() {
	c.next++
	c.live[fmt.Sprintf("inc-%06d", c.next)] = Item[string]{
		Value: c.record(c.next),
	}
}

// openIDs returns the IDs of the open incidents in a stable order.
func (c *churn) openIDs() []string {
	var out []string
	for id, item := range c.live {
		if item.Expires.IsZero() {
			out = append(out, id)
		}
	}
	sortStrings(out)
	return out
}

// step simulates churnStep hours: new incidents, resolves, revisions,
// digest changes and timeline entries, then one compactor pass.
func (c *churn) step(pass func()) {
	c.clock.Advance(churnStep * time.Hour)
	now := c.clock.Now()
	for i := 0; i < 3*churnStep; i++ {
		c.open()
	}
	ids := c.openIDs()
	for i := 0; i < 3*churnStep; i++ { // resolve: kept 7 days
		id := ids[c.rng.Intn(len(ids))]
		c.live[id] = Item[string]{
			Value: c.record(c.rng.Intn(1000)), Expires: now.Add(7 * day),
		}
	}
	for i := 0; i < 20*churnStep; i++ { // revise an open incident
		id := ids[c.rng.Intn(len(ids))]
		if c.live[id].Expires.IsZero() {
			c.live[id] = Item[string]{Value: c.record(c.rng.Intn(1000))}
		}
	}
	for id, item := range c.live { // the manager forgets expired ones
		if !item.Expires.IsZero() && !item.Expires.After(now) {
			delete(c.live, id)
		}
	}
	_, err := c.incidents.Replace(c.live)
	require.NoError(c.t, err)
	for i := 0; i < 100*churnStep; i++ {
		key := fmt.Sprintf("pod/ns/p-%d", c.rng.Intn(churnFingerprints))
		c.prints404[key] = Item[string]{Value: fmt.Sprint(c.rng.Int())}
	}
	_, err = c.prints.Replace(c.prints404)
	require.NoError(c.t, err)
	c.timeline(now)
	pass()
}

func (c *churn) timeline(now time.Time) {
	text := strings.Repeat("t", churnTimelineEntryText)
	n := churnTimelinePerHour * churnStep
	entries := make([]Entry[string], n)
	for i := range entries {
		entries[i] = Entry[string]{
			Entity: fmt.Sprintf("pod/ns/e-%03d", c.rng.Intn(churnEntities)),
			At: now.Add(-time.Duration(n-i) * time.Hour * churnStep /
				time.Duration(n)),
			Value: text,
		}
	}
	require.NoError(c.t, TimelineLog[string](c.store).AppendAll(entries))
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// compactEvery returns a pass function that runs the production
// compactor, the way its 15 minute ticker would.
func compactEvery(t *testing.T, s *Store) func() {
	c := NewCompactor(s, DefaultPolicy())
	return func() {
		_, err := c.Pass(context.Background())
		require.NoError(t, err)
	}
}

// After two weeks of churn the file must hold about what the
// retention windows keep, not two weeks of history, and it must open
// quickly.
func TestChurnKeepsStateFileBoundedByLiveContent(t *testing.T) {
	clock := newFakeClock()
	path := filepath.Join(t.TempDir(), "state.db")
	s, err := Open(path, Options{Now: clock.Now})
	require.NoError(t, err)
	claim(t, s)
	c := newChurn(t, s, clock)
	pass := compactEvery(t, s)

	for h := 0; h < 14*24/churnStep; h++ {
		c.step(pass)
	}

	sizes, err := s.LogicalSize()
	require.NoError(t, err)
	perEntry := int64(churnTimelineEntryText + 80)
	week := int64(7 * 24 * churnTimelinePerHour)
	assert.LessOrEqual(t, sizes[Timeline], week*perEntry*11/10,
		"timeline holds more than the days anything reads")
	size, _ := fileSize(path)
	_, free := s.fileUsage()
	t.Logf("after 14 days: file=%d free=%d logical=%d timeline=%d",
		size, free, total(sizes), sizes[Timeline])
	assert.Less(t, size, int64(45<<20), "the file is not bounded")
	assert.Less(t, size, 3*total(sizes), "most of the file is waste")
	require.NoError(t, s.Close())

	began := time.Now()
	reopened, err := Open(path, Options{Now: clock.Now, DeferRepair: true})
	require.NoError(t, err)
	defer reopened.Close()
	claim(t, reopened)

	took := time.Since(began)
	t.Logf("open+claim of %d bytes took %s", size, took)
	assert.Less(t, took, 10*time.Second)
}
