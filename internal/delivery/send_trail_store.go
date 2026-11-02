package delivery

import (
	"sort"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/notification"
)

// trailRecordVersion is the persisted trail layout. A record with
// another version is dropped at load, never misread.
const trailRecordVersion = 1

// TrailStore is the optional persistence port of the send trail. An
// OutboxStore that also implements it keeps the trail across restarts:
// the outbox writer saves it with its own batches.
type TrailStore interface {
	// LoadTrails returns the saved trails of the incidents still known.
	LoadTrails() ([]TrailRecord, error)
	// WriteTrails stores puts, then removes the trails named by removes.
	WriteTrails(puts []TrailRecord, removes []string) error
}

// TrailRecord is the saved trail of one incident.
type TrailRecord struct {
	Version int `json:"version"`
	// Key is the incident ID.
	Key string `json:"key"`
	// Attempts are the newest attempts, oldest first.
	Attempts []SendAttempt `json:"attempts"`
	// Sent is the text of the latest message each provider accepted,
	// by provider name. Records saved before it existed have none.
	Sent map[string]SentText `json:"sent,omitempty"`
}

// last is when the record's newest attempt was made.
func (r TrailRecord) last() int64 {
	if len(r.Attempts) == 0 {
		return 0
	}
	return r.Attempts[len(r.Attempts)-1].At.UnixNano()
}

// restore puts saved records, least recently told first, behind what
// this session already noted: they are older. Records of another
// layout, and of summaries, are deleted by the next write, which is
// woken at once. The bounds
// apply to the merged trail.
func (t *sendTrail) restore(records []TrailRecord) {
	sort.SliceStable(records, func(i, j int) bool {
		return records[i].last() < records[j].last()
	})
	t.mu.Lock()
	if t.byKey == nil {
		t.byKey = map[string][]SendAttempt{}
	}
	var order []string
	for _, r := range records {
		if r.Key == "" {
			continue
		}
		if (notification.Message{Key: r.Key}).IsSummary() {
			// Saved before summaries were left out of the trail.
			t.markLocked(r.Key, false)
			continue
		}
		if r.Version != trailRecordVersion || len(r.Attempts) == 0 {
			t.markLocked(r.Key, false)
			continue
		}
		newer, seen := t.byKey[r.Key]
		if seen {
			t.markLocked(r.Key, true)
		} else {
			order = append(order, r.Key)
		}
		t.byKey[r.Key] = keepNewest(append(
			append([]SendAttempt(nil), r.Attempts...), newer...))
		t.restoreTextLocked(r.Key, r.Sent)
	}
	t.order = append(order, t.order...)
	t.boundLocked()
	changed, dirty := t.changed, len(t.dirty) > 0
	t.mu.Unlock()
	// Deletions found here are written now, not at the next send.
	if dirty && changed != nil {
		changed()
	}
}

// take returns the changes since the last write and forgets them.
func (t *sendTrail) take() (puts []TrailRecord, removes []string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for key, live := range t.dirty {
		list, ok := t.byKey[key]
		if !live || !ok {
			removes = append(removes, key)
			continue
		}
		puts = append(puts, TrailRecord{Version: trailRecordVersion,
			Key: key, Attempts: append([]SendAttempt(nil), list...),
			Sent: t.textsLocked(key)})
	}
	t.dirty = nil
	return puts, removes
}

// retake marks a failed batch for the next write again, unless a newer
// change to the same key is already pending.
func (t *sendTrail) retake(puts []TrailRecord, removes []string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	keys := append([]string(nil), removes...)
	for _, r := range puts {
		keys = append(keys, r.Key)
	}
	for _, key := range keys {
		if _, pending := t.dirty[key]; !pending {
			_, live := t.byKey[key]
			t.markLocked(key, live)
		}
	}
}

// onChange sets who is woken when the trail changes.
// Changes already waiting (the deletions restore found) wake it at once.
func (t *sendTrail) onChange(changed func()) {
	t.mu.Lock()
	t.changed = changed
	dirty := len(t.dirty) > 0
	t.mu.Unlock()
	if dirty && changed != nil {
		changed()
	}
}

// loadTrail loads the saved trail when store keeps one. A failed load
// is logged: the trail is what the dashboard shows, never needed to
// deliver.
func loadTrail(store OutboxStore) (TrailStore, []TrailRecord) {
	trails, ok := store.(TrailStore)
	if !ok {
		return nil, nil
	}
	records, err := trails.LoadTrails()
	if err != nil {
		klog.ErrorS(err, "load delivery trail",
			"component", "delivery", "operation", "trail")
		return trails, nil
	}
	return trails, records
}

// attachTrailLocked restores the loaded trail and lets the outbox writer
// save it from now on. It runs once, with the outbox that won the
// attach, so a racing attach never restores the trail twice or steals
// its writer. The caller holds m.mu.
func (m *Manager) attachTrailLocked(
	trails TrailStore, records []TrailRecord, box *outbox,
) {
	if trails == nil {
		return
	}
	m.trail.restore(records)
	box.trails, box.trail = trails, &m.trail
	m.trail.onChange(box.signal)
}

// flushTrail writes the trail changes. A failed batch is marked again,
// so the next flush retries it.
func (o *outbox) flushTrail() (ok, wrote bool) {
	if o.trail == nil {
		return true, false
	}
	o.mu.Lock()
	sealed := o.sealed
	o.mu.Unlock()
	if sealed {
		return true, false
	}
	puts, removes := o.trail.take()
	if len(puts) == 0 && len(removes) == 0 {
		return true, false
	}
	written := false
	defer func() {
		if !written {
			o.trail.retake(puts, removes)
		}
	}()
	if err := o.trails.WriteTrails(puts, removes); err != nil {
		klog.ErrorS(err, "write delivery trail",
			"component", "delivery", "operation", "trail")
		return false, true
	}
	written = true
	return true, true
}
