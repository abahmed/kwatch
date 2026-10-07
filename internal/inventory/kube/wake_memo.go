package kube

import (
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// wakeMemo remembers the last scan, so the cluster is scanned once per
// wakeRecompute and not once per pod evaluated.
var wakeMemo = &memo{}

type memo struct {
	mu     sync.Mutex
	reader inventory.Reader
	at     time.Time
	wake   Wake
	found  bool
}

// get returns the remembered scan while it is fresh for the same reader,
// and scans again otherwise.
func (m *memo) get(
	r inventory.Reader, now time.Time,
	scan func(inventory.Reader, time.Time) (Wake, bool),
) (Wake, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	age := now.Sub(m.at)
	if m.reader != nil && sameReader(m.reader, r) &&
		age >= 0 && age < wakeRecompute {
		return m.wake, m.found
	}
	m.reader, m.at = r, now
	m.wake, m.found = scan(r, now)
	return m.wake, m.found
}

// sameReader compares readers without panicking on one that cannot be
// compared.
func sameReader(a, b inventory.Reader) (same bool) {
	defer func() {
		if recover() != nil {
			same = false
		}
	}()
	return a == b
}
