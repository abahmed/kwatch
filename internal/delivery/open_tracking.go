package delivery

import (
	"github.com/abahmed/kwatch/internal/notification"
)

// A provider can miss a conversation's announcement: the queue overflowed,
// a newer revision replaced it before it was sent, the provider rejected
// it, or the hourly budget folded it into an overflow summary. A later
// bare update or resolve then reaches a channel that never heard of the
// incident.
//
// Delivery remembers, per provider and key, that the announcement did not
// arrive. The next update is sent as the announcement, and a resolve is
// sent as one combined "opened and resolved" message. The memory is only
// kept for this process; a restart forgets it.

// openFate is what happened to a conversation's announcement.
type openFate uint8

const (
	// openLost: the announcement never reached the provider.
	openLost openFate = iota + 1
	// openFolded: the hourly budget folded the announcement into an
	// overflow summary, so the conversation's later messages are folded
	// too.
	openFolded
)

// maxTrackedOpens bounds the remembered announcements per provider.
const maxTrackedOpens = 4096

// lostOpeningNote is appended to a resolve whose announcement was lost
// when the composer did not attach the announcement itself.
const lostOpeningNote = "Its opening notification did not reach " +
	"this channel."

// openEntry is a remembered fate with its insertion order.
type openEntry struct {
	fate openFate
	seq  uint64
}

func (m *Manager) setOpenFate(provider, key string, fate openFate) {
	m.opensMu.Lock()
	defer m.opensMu.Unlock()
	if m.opens == nil {
		m.opens = make(map[string]map[string]openEntry)
	}
	keys := m.opens[provider]
	if keys == nil {
		keys = make(map[string]openEntry)
		m.opens[provider] = keys
	}
	if _, known := keys[key]; !known && len(keys) >= maxTrackedOpens {
		delete(keys, evictableOpen(keys))
	}
	m.openSeq++
	keys[key] = openEntry{fate: fate, seq: m.openSeq}
}

// evictableOpen picks the oldest lost entry, or the oldest entry when
// none is lost. A folded marker is kept for as long as possible: without
// it a later update or resolve would reach a channel that was never told
// about the conversation.
func evictableOpen(keys map[string]openEntry) string {
	var victim string
	var best openEntry
	for key, entry := range keys {
		if victim == "" || evictsBefore(entry, best) {
			victim, best = key, entry
		}
	}
	return victim
}

func evictsBefore(a, b openEntry) bool {
	if (a.fate == openLost) != (b.fate == openLost) {
		return a.fate == openLost
	}
	return a.seq < b.seq
}

func (m *Manager) openFateOf(provider, key string) openFate {
	m.opensMu.Lock()
	defer m.opensMu.Unlock()
	return m.opens[provider][key].fate
}

func (m *Manager) forgetOpen(provider, key string) {
	m.opensMu.Lock()
	defer m.opensMu.Unlock()
	delete(m.opens[provider], key)
}

func (m *Manager) markOpenLost(provider, key string) {
	if m.openFateOf(provider, key) == 0 {
		m.setOpenFate(provider, key, openLost)
	}
}

// openFailed records that a job left the provider without being sent.
// Only an announcement matters: a lost update leaves the conversation
// announced.
func (m *Manager) openFailed(entry *providerEntry, job deliverJob) {
	if job.opens() {
		m.markOpenLost(entry.lookupName(), job.key())
	}
}

// openSettled records that the provider accepted a message of the
// conversation, which therefore is announced now (or over).
func (m *Manager) openSettled(entry *providerEntry, job deliverJob) {
	if job.kind == jobIncident && job.incident != nil {
		m.forgetOpen(entry.lookupName(), job.key())
	}
}

// makeUpForLostOpen rewrites a message whose conversation the provider
// never saw announced. An update already describes the whole current
// state; a provider without a thread for the key posts it as a new
// conversation. A resolve is combined with the announcement, so the
// channel learns what opened and what fixed it in one message.
func (m *Manager) makeUpForLostOpen(
	entry *providerEntry, msg notification.Message,
) notification.Message {
	if msg.IsOpening() ||
		m.openFateOf(entry.lookupName(), msg.Key) != openLost {
		msg.Opening = nil
		return msg
	}
	if msg.Resolved() {
		msg = combinedResolve(msg)
	}
	msg.Opening = nil
	return msg
}

// combinedResolve is one "opened and resolved" message. The resolve
// keeps its marker as the first character of the note.
func combinedResolve(m notification.Message) notification.Message {
	if m.Opening != nil {
		m.Note = m.NoteText() + "\n\nIt opened without a notification " +
			"reaching this channel:\n" + m.Opening.NoteText()
		return m
	}
	m.Note = m.NoteText() + "\n\n" + lostOpeningNote
	return m
}
