package inventory

// Notes and object identity: which notes belong to the object an entity
// currently is.

func (m *Model) note(observation Observation) Update {
	rec := m.recordFor(observation.Entity)
	note := observation.Note
	if m.aboutOlderObject(rec, note) {
		return Update{}
	}
	if note.At.IsZero() {
		note.At = observation.At
	}
	if note.FirstSeen.IsZero() || note.FirstSeen.After(note.At) {
		note.FirstSeen = note.At
	}
	key := noteKey{note.Source, note.Reason, note.UID}
	rec.takeSame(&note)
	if note.Origin != "" {
		if rec.noteParts == nil {
			rec.noteParts = make(map[noteKey][]originCount)
		}
		rec.noteParts[key], note.Count = addOrigin(
			rec.noteParts[key], note.Origin, note.Count)
	} else {
		// A note without an origin replaces the counted one; its old
		// parts must not be added to if an origin returns later.
		delete(rec.noteParts, key)
	}
	rec.notes = append(rec.notes, note)
	if over := len(rec.notes) - DefaultMaxNotesPerEntity; over > 0 {
		rec.notes = append(rec.notes[:0:0], rec.notes[over:]...)
		rec.dropOrphanParts()
	}
	return Update{Touched: []EntityID{observation.Entity}}
}

// takeSame removes the note that note replaces (same source, reason and
// UID) and carries its first and last times over.
func (r *record) takeSame(note *Note) {
	for i, existing := range r.notes {
		if existing.Source == note.Source && existing.Reason == note.Reason &&
			existing.UID == note.UID {
			note.FirstSeen = earliest(existing.FirstSeen, note.FirstSeen)
			if existing.At.After(note.At) {
				note.At = existing.At
			}
			r.notes = append(r.notes[:i], r.notes[i+1:]...)
			return
		}
	}
}

// adoptUID records the UID of the object the entity now is. A different
// UID than before is a different object under the same name, so the old
// second identifier goes with it. The second identifier is stored before
// notes are filtered: an early kubelet event about a static pod carries
// the config hash, and it must not be purged as a stranger's note.
func (r *record) adoptUID(uid, altUID string) {
	changed := uid != "" && r.entity.UID != "" && r.entity.UID != uid
	if changed || altUID != "" {
		r.altUID = altUID
	}
	if uid == "" {
		return
	}
	switch {
	case changed:
		r.startNewObject(uid)
	case r.entity.UID == "":
		// Notes may have arrived before the first observation; those
		// about an older object with this name are not evidence.
		r.dropNotesAbout(uid, true)
	}
	r.entity.UID = uid
}

// aboutOlderObject reports a note for an object other than the one the
// entity currently is. An entity that is gone accepts it: the note may be
// about the object about to appear under the same name.
func (m *Model) aboutOlderObject(rec *record, note Note) bool {
	if !rec.present || note.UID == "" {
		return false
	}
	uid := m.currentUID(rec)
	return uid != "" && note.UID != uid && note.UID != m.currentAltUID(rec)
}

// currentAltUID is the second identifier of the object an entity is,
// found the same way as currentUID.
func (m *Model) currentAltUID(rec *record) string {
	if rec.altUID != "" {
		return rec.altUID
	}
	if rec.entity.UID != "" {
		return ""
	}
	for key, targets := range rec.relations {
		if key.relation != PartOf {
			continue
		}
		for _, target := range targets {
			if parent := m.records[target]; parent != nil &&
				parent.present && parent.altUID != "" {
				return parent.altUID
			}
		}
	}
	return ""
}

// currentUID is the UID of the object an entity is. A part of another
// entity, such as a container of a pod, has none of its own and is the
// object of the entity it is part of.
func (m *Model) currentUID(rec *record) string {
	if rec.entity.UID != "" {
		return rec.entity.UID
	}
	for key, targets := range rec.relations {
		if key.relation != PartOf {
			continue
		}
		for _, target := range targets {
			if parent := m.records[target]; parent != nil &&
				parent.present && parent.entity.UID != "" {
				return parent.entity.UID
			}
		}
	}
	return ""
}

// startNewObject forgets what was said about the previous object that had
// this entity's name: its notes and health marks.
func (r *record) startNewObject(uid string) {
	r.dropNotesAbout(uid, false)
	r.health = nil
}

// dropNotesAbout keeps only the notes about the object with this UID (or
// its second identifier), and those without a UID when keepBlank is set.
func (r *record) dropNotesAbout(uid string, keepBlank bool) {
	kept := r.notes[:0:0]
	for _, note := range r.notes {
		if note.UID == uid || (r.altUID != "" && note.UID == r.altUID) ||
			(keepBlank && note.UID == "") {
			kept = append(kept, note)
		}
	}
	r.notes = kept
	r.dropOrphanParts()
}

// dropOrphanParts forgets the origins of notes that no longer exist.
func (r *record) dropOrphanParts() {
	for key := range r.noteParts {
		found := false
		for _, note := range r.notes {
			if (noteKey{note.Source, note.Reason, note.UID}) == key {
				found = true
				break
			}
		}
		if !found {
			delete(r.noteParts, key)
		}
	}
}
