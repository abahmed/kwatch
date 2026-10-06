package incident

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
)

// newID returns a short, readable ID such as "inc-20261001-7f3a-0007":
// the UTC day the incident opened, the store's nonce and a sequence
// number that only grows. IDs never change, so delivery keeps one
// conversation per incident even when its cause is revised. Restore
// moves the sequence past every restored ID. The nonce keeps IDs unique
// when the sequence starts over: after a store reset, or on a new empty
// volume after a reschedule, the same day and sequence get a new nonce.
func (m *Manager) newID(now time.Time) string {
	m.seq++
	return fmt.Sprintf("inc-%s-%s-%04d",
		now.UTC().Format("20060102"), m.nonce, m.seq)
}

// hashBytes is how much of a SHA-256 sum names an alert or fingerprints
// an incident: 8 bytes, 16 hex characters, short enough to read in a log
// and wide enough that two incidents of one cluster never collide.
const hashBytes = 8

// alertKey is the stable alert identity of a root failing in a mode,
// such as "alert-3f9a0c27d41e8b65": a hash of the content, so the same
// failure gets the same key on every store. Paging providers add the
// cluster name (notification.Message.AlertKey).
func alertKey(root inventory.EntityID, mode detection.Mode) string {
	sum := sha256.Sum256([]byte(root.String() + "|" + string(mode)))
	return "alert-" + hex.EncodeToString(sum[:hashBytes])
}

// freeAlertKey is the alert key of a new incident: the key of its root
// and mode, unless a live incident holds it. A revised incident keeps the
// key of its first root, so a later incident on that root and mode would
// share its alert: each would update and resolve the other's. The new one
// gets a numbered suffix instead.
func (m *Manager) freeAlertKey(p *Incident) string {
	base := alertKey(p.Root, p.Mode)
	key := base
	for n := 2; m.liveAlertKey(p, key); n++ {
		key = fmt.Sprintf("%s-%d", base, n)
	}
	return key
}

// liveAlertKey reports whether an incident other than p, not yet
// resolved, holds the alert key.
func (m *Manager) liveAlertKey(p *Incident, key string) bool {
	for _, other := range m.incidents {
		if other != p && other.State != Resolved && other.AlertKey == key {
			return true
		}
	}
	return false
}

// idNonceBytes is the random part of an ID nonce: 2 bytes, 4 hex
// characters, so two stores of one cluster collide once in 65536.
const idNonceBytes = 2

// randomNonce returns a fresh ID nonce such as "7f3a".
func randomNonce() string {
	b := make([]byte, idNonceBytes)
	// crypto/rand.Read never returns an error since Go 1.24.
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// idNonce returns the nonce of an ID made by newID, or "" for an ID
// without one, such as those made before nonces existed.
func idNonce(id string) string {
	parts := strings.Split(id, "-")
	if len(parts) != 4 || parts[0] != "inc" ||
		len(parts[2]) != 2*idNonceBytes {
		return ""
	}
	if _, err := hex.DecodeString(parts[2]); err != nil {
		return ""
	}
	return parts[2]
}

// idSequence returns the sequence number of an ID made by newID, or zero
// for any other ID.
func idSequence(id string) int {
	n, err := strconv.Atoi(id[strings.LastIndexByte(id, '-')+1:])
	if err != nil {
		return 0
	}
	return n
}

// lookup returns the latest incident for root, or nil.
func (m *Manager) lookup(root inventory.EntityID) *Incident {
	id, ok := m.byRoot[root.String()]
	if !ok {
		return nil
	}
	return m.incidents[id]
}

// index makes p the incident of its root.
func (m *Manager) index(p *Incident) {
	m.byRoot[p.Root.String()] = p.ID
}

// unindex drops p's root entry when it still points at p.
func (m *Manager) unindex(p *Incident) {
	if m.byRoot[p.Root.String()] == p.ID {
		delete(m.byRoot, p.Root.String())
	}
}

// indexRestored indexes a restored incident unless its root already has
// a better one: a live incident wins over a resolved one, otherwise the
// one opened last wins.
func (m *Manager) indexRestored(p *Incident) {
	held := m.lookup(p.Root)
	if held == nil {
		m.index(p)
		return
	}
	pLive, heldLive := p.State != Resolved, held.State != Resolved
	if pLive != heldLive {
		if pLive {
			m.index(p)
		}
		return
	}
	if p.Opened.After(held.Opened) {
		m.index(p)
	}
}
