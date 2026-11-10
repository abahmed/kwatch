package incident

// The ID sequence only grows, also across restarts. The records alone
// cannot say how far it got: an incident that is forgotten (a blip that
// recurred, a finding that moved before anyone was told) takes its
// number with it, and a restart that counted only the records left
// would give that number out again, to an incident whose messages then
// land in the thread and the audit history of the old one. So the
// highest number ever used is saved too, on one record.

// SequenceMark is the part of a Record that carries the ID sequence.
type SequenceMark struct {
	// SeqHigh is the highest sequence number of any ID this store gave
	// out, when it is higher than the number of every saved ID. Only
	// the record with the highest number carries it, so opening an
	// incident rewrites one record, not all of them. Older records
	// have none and restore as before.
	SeqHigh int `json:",omitempty"`
}

// markSequence writes the sequence's high-water mark on the record with
// the highest sequence number, when a forgotten incident had a higher
// one. With no record left there is nothing to carry it, and nothing
// to protect: a store without records starts with a new nonce (newID).
func (m *Manager) markSequence(records []Record) []Record {
	carrier, highest := -1, 0
	for i, r := range records {
		if seq := idSequence(r.ID); carrier < 0 || seq > highest {
			carrier, highest = i, seq
		}
	}
	if carrier >= 0 && m.seq > highest {
		records[carrier].SeqHigh = m.seq
	}
	return records
}

// restoreSequence moves the sequence past every restored ID and past
// the saved high-water mark, and takes the nonce of the newest restored
// ID, so new IDs continue the store's own series.
func (m *Manager) restoreSequence(records []Record) {
	newest := 0
	for _, r := range records {
		if seq := idSequence(r.ID); seq > newest {
			newest = seq
			if nonce := idNonce(r.ID); nonce != "" {
				m.nonce = nonce
			}
		}
		m.seq = max(m.seq, r.SeqHigh)
	}
	m.seq = max(m.seq, newest)
}
