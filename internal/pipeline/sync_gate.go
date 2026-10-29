package pipeline

import (
	"sync"

	"github.com/abahmed/kwatch/internal/inventory"
)

// SyncGate answers whether a kind is synced for decisions. A source
// reports a kind synced once it has handed its whole first list to
// Engine.Submit, but those observations reach the model only when the
// engine drains its queue. Read live, the source can say "synced" while
// part of the list still waits in the queue, and a detector concludes
// that a listed object does not exist (a webhook's Service, a built-in
// PriorityClass) for the first seconds after a start.
//
// The gate counts a kind as synced only when it was already synced as a
// drain began: everything listed before that point is in the model by
// the time a detector reads it. A kind asked about earlier is noted as
// unsynced, so the engine evaluates the entity again once it is.
type SyncGate struct {
	live func(inventory.Kind) bool

	mu        sync.Mutex
	confirmed map[inventory.Kind]bool
	asked     map[inventory.Kind]bool
}

// NewSyncGate returns a gate over the source's live sync state.
func NewSyncGate(live func(inventory.Kind) bool) *SyncGate {
	return &SyncGate{
		live:      live,
		confirmed: map[inventory.Kind]bool{},
		asked:     map[inventory.Kind]bool{},
	}
}

// Synced reports whether the kind was synced when the latest drain began
// and still is: a kind that stops being watched is unsynced at once.
func (g *SyncGate) Synced(kind inventory.Kind) bool {
	g.mu.Lock()
	confirmed := g.confirmed[kind]
	if !confirmed {
		g.asked[kind] = true
	}
	g.mu.Unlock()
	return confirmed && g.live(kind)
}

// Advance runs just before the engine drains its queue: the kinds synced
// now have submitted their whole list, which the drain applies.
func (g *SyncGate) Advance() {
	g.mu.Lock()
	defer g.mu.Unlock()
	for kind := range g.asked {
		if g.live(kind) {
			g.confirmed[kind] = true
			delete(g.asked, kind)
		}
	}
}
