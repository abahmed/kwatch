package pipeline

import (
	"slices"
	"sort"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/storage"
)

// maxPinnedPerEntity bounds the older changes a restart pins for one
// entity: the newest ones are kept.
const maxPinnedPerEntity = inventory.DefaultMaxChangesPerEntity

// pinStore is the part of a store a restart reads older changes from.
type pinStore interface {
	// ScanChanges visits the changes with since <= At < until, oldest
	// first.
	ScanChanges(since, until time.Time, visit func(inventory.Change)) error
}

var _ pinStore = (*diskIncidents)(nil)

// ScanChanges implements pinStore. Records of an unknown version and
// records without an entity are skipped.
func (d *diskIncidents) ScanChanges(
	since, until time.Time, visit func(inventory.Change),
) error {
	return storage.ChangeLog[changeRecord](d.store).Range(changeStream,
		since, until, func(_ time.Time, record changeRecord) error {
			if record.V >= 1 && record.V <= recordVersion &&
				!record.Change.Entity.IsZero() {
				visit(record.Change)
			}
			return nil
		})
}

// pinClaim is how far back an entity's changes are pinned, and for
// which open incidents.
type pinClaim struct {
	from time.Time
	ids  []string
}

func (c pinClaim) with(from time.Time, id string) pinClaim {
	if c.from.IsZero() || from.Before(c.from) {
		c.from = from
	}
	if !slices.Contains(c.ids, id) {
		c.ids = append(c.ids, id)
	}
	return c
}

// pinScope is what the open incidents pin: the entities they name
// (roots, causes, chains, members) and, as the model has no relations
// yet at restore, every persisted entity of their namespaces, which
// holds the workloads that own them and the config and services those
// reference.
type pinScope struct {
	entities map[inventory.EntityID]pinClaim
	spaces   map[string]pinClaim
}

// openPinScope is the pin scope of the open incidents in records.
func openPinScope(records []incident.Record) pinScope {
	s := pinScope{entities: map[inventory.EntityID]pinClaim{},
		spaces: map[string]pinClaim{}}
	for _, r := range records {
		if r.State == incident.Resolved || r.Opened.IsZero() {
			continue
		}
		from := r.Opened.Add(-inventory.MaxEffectWindow)
		if c := r.Cause; c != nil && c.Change != nil &&
			c.Change.At.Before(from) {
			from = c.Change.At
		}
		for _, id := range recordEntities(r) {
			s.entities[id] = s.entities[id].with(from, r.ID)
			if id.Namespace != "" {
				s.spaces[id.Namespace] =
					s.spaces[id.Namespace].with(from, r.ID)
			}
		}
	}
	return s
}

// recordEntities are the entities a record names.
func recordEntities(r incident.Record) []inventory.EntityID {
	out := []inventory.EntityID{r.Root}
	for c := r.Cause; c != nil; c = c.Rival {
		out = append(out, causeEntities(c)...)
	}
	for _, event := range r.Timeline {
		if event.Entity != nil {
			out = append(out, *event.Entity)
		}
	}
	return slices.DeleteFunc(out, inventory.EntityID.IsZero)
}

func causeEntities(c *rootcause.CauseRecord) []inventory.EntityID {
	out := append([]inventory.EntityID{c.Root}, c.Chain...)
	if c.Change != nil {
		out = append(out, c.Change.Entity)
	}
	return out
}

// claim is the claim on change's entity, or ok false.
func (s pinScope) claim(change inventory.Change) (pinClaim, bool) {
	claim, ok := s.entities[change.Entity]
	if !ok && change.Entity.Namespace != "" {
		claim, ok = s.spaces[change.Entity.Namespace]
	}
	if !ok || change.At.Before(claim.from) {
		return pinClaim{}, false
	}
	return claim, true
}

// pinnedChanges reads the changes before until that the open incidents
// pin, at most maxPinnedPerEntity per entity and
// inventory.DefaultMaxPinnedChanges in all, the newest kept. It notes
// which incidents hold each pinned entity, so their resolve unpins it.
func (h *historyRecorder) pinnedChanges(until time.Time) []inventory.Change {
	store, ok := h.store.(pinStore)
	if !ok || h.incidents == nil || until.IsZero() {
		return nil
	}
	scope := openPinScope(h.incidents.Export())
	if len(scope.entities) == 0 {
		return nil
	}
	since := h.now().Add(-inventory.PinnedRetention)
	byEntity := map[inventory.EntityID][]inventory.Change{}
	err := store.ScanChanges(since, until, func(change inventory.Change) {
		claim, ok := scope.claim(change)
		if !ok {
			return
		}
		id := change.Entity
		byEntity[id] = append(byEntity[id], change)
		if over := len(byEntity[id]) - maxPinnedPerEntity; over > 0 {
			byEntity[id] = byEntity[id][over:]
		}
		h.pinnedBy[change.Entity] = claim.ids
	})
	if err != nil {
		klog.ErrorS(err, "pipeline: restore pinned changes",
			"component", "pipeline", "operation", "restore")
	}
	var out []inventory.Change
	for _, changes := range byEntity {
		out = append(out, changes...)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].At.Before(out[j].At)
	})
	if over := len(out) - inventory.DefaultMaxPinnedChanges; over > 0 {
		out = out[over:]
	}
	return out
}

// releasePins unpins the entities whose incidents are all resolved.
func (h *historyRecorder) releasePins(now time.Time) {
	if len(h.pinnedBy) == 0 || h.incidents == nil {
		return
	}
	open := map[string]bool{}
	for _, p := range h.incidents.Incidents() {
		if p.State != incident.Resolved {
			open[p.ID] = true
		}
	}
	var done []inventory.EntityID
	held := func(id string) bool { return open[id] }
	for id, ids := range h.pinnedBy {
		if !slices.ContainsFunc(ids, held) {
			done = append(done, id)
			delete(h.pinnedBy, id)
		}
	}
	if len(done) > 0 && !now.IsZero() {
		h.model.Unpin(done, now.Add(-inventory.ChangeRetention))
	}
}
