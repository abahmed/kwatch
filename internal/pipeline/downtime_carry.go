package pipeline

import (
	"maps"
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
)

// carriedKind is the stored baseline of one kind that had not synced
// when the downtime comparison ran.
type carriedKind struct {
	// at is the snapshot time the entries were taken at.
	at      string
	entries map[string]string
}

// carriedKinds keep the stored fingerprints of kinds that had not
// synced at the downtime comparison, such as an optional kind still
// listing when the sync wait ended. The model may hold only part of
// such a kind, so comparing it would miss changes and saving it would
// erase the baseline. Each save writes the stored entries back instead,
// and the kind is compared once it syncs, dated at its own snapshot.
// A kind that never syncs (a disabled watch) keeps its baseline.
type carriedKinds map[inventory.Kind]carriedKind

// splitSaved returns the saved fingerprints to compare now, those of
// synced kinds and the snapshot times, and carries the rest: the
// entries and the time key of every unsynced kind. A nil synced treats
// every kind as synced.
func splitSaved(
	saved map[string]string, synced func(inventory.Kind) bool,
) (map[string]string, carriedKinds) {
	compare := map[string]string{}
	carried := carriedKinds{}
	for key, value := range saved {
		kind, entry := savedKind(key)
		if kind == "" || kindSynced(synced, kind) {
			compare[key] = value
			continue
		}
		c, ok := carried[kind]
		if !ok {
			c = carriedKind{
				at:      snapshotOf(saved, kind),
				entries: map[string]string{},
			}
			carried[kind] = c
		}
		if entry {
			c.entries[key] = value
		}
	}
	return compare, carried
}

// savedKind is the kind a saved key belongs to, an entity's or a kind
// time key's, and whether the key is an entity's. Other keys have no
// kind.
func savedKind(key string) (kind inventory.Kind, entry bool) {
	if id, ok := inventory.ParseEntityID(key); ok {
		return id.Kind, true
	}
	if name, ok := strings.CutPrefix(key, kindTimePrefix); ok {
		return inventory.Kind(name), false
	}
	return "", false
}

// snapshotOf is the snapshot time of kind's saved fingerprints: its own
// when it has one, otherwise the snapshot's.
func snapshotOf(saved map[string]string, kind inventory.Kind) string {
	if at, ok := saved[kindTimePrefix+string(kind)]; ok {
		return at
	}
	return saved[snapshotTimeKey]
}

// due removes the carried kinds that have synced and returns their
// fingerprints as one saved set, or nil when none has. The caller must
// ask before it drains the inbox: a kind reads as synced only after its
// source submitted the whole initial list.
func (c carriedKinds) due(
	synced func(inventory.Kind) bool,
) map[string]string {
	var out map[string]string
	for kind, carried := range c {
		if !kindSynced(synced, kind) {
			continue
		}
		if out == nil {
			out = map[string]string{}
		}
		maps.Copy(out, carried.entries)
		out[kindTimePrefix+string(kind)] = carried.at
		delete(c, kind)
	}
	return out
}

// keep replaces, in fingerprints about to be saved, every entry and the
// time key of a carried kind with the stored ones.
func (c carriedKinds) keep(fingerprints map[string]any) {
	if len(c) == 0 {
		return
	}
	for key := range fingerprints {
		kind, _ := savedKind(key)
		if _, carried := c[kind]; carried {
			delete(fingerprints, key)
		}
	}
	for kind, carried := range c {
		for key, value := range carried.entries {
			fingerprints[key] = value
		}
		if carried.at != "" {
			fingerprints[kindTimePrefix+string(kind)] = carried.at
		}
	}
}
