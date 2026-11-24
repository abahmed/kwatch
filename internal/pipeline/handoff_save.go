package pipeline

import "github.com/abahmed/kwatch/internal/incident"

// Delivery records an announcement in its outbox as soon as it has it,
// while the incident record that says it was announced waits for the
// next batched save (writeBatch). A process killed in between restores
// the incident as not announced yet and announces it again, and the
// outbox sends the first message too. So the record saying it was
// announced is written, and waited for, before the announcement is
// handed to delivery. Only that incident's record changes in what is
// written: the others stay as last saved, so one decided in the same
// tick but not handed over yet (held for its investigation) is not
// written as announced before it is.

// saveBeforeHandoff writes the record of incident id, as it is now,
// over the records last saved, and waits for the write. A failed write
// is retried with the next batch; the announcement goes out regardless.
func (a *announcer) saveBeforeHandoff(id string) {
	s := a.storage
	if s == nil || s.incidents == nil {
		return
	}
	current, ok := a.incidents.ExportOf(id)
	if !ok {
		return
	}
	records := withRecord(s.last, current)
	if s.openOnly {
		records = openAnnounced(records, a.collect.Startup.Baselined)
	}
	s.last = records
	s.incidents.writeNow(storeSnapshot{incidents: records,
		hasIncidents: true})
}

// withRecord is a copy of records with r in place of the record of the
// same ID, or added at the end.
func withRecord(
	records []incident.Record, r incident.Record,
) []incident.Record {
	out := make([]incident.Record, 0, len(records)+1)
	added := false
	for _, old := range records {
		if old.ID == r.ID {
			old, added = r, true
		}
		out = append(out, old)
	}
	if !added {
		out = append(out, r)
	}
	return out
}
