package app

import (
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/insight"
)

// activeKeyTTL bounds how stale the active-incident view used for evidence may
// be. Evidence is advisory, and one rebuild per interval is the difference
// between a constant-time lookup and a full scan per dependency.
const activeKeyTTL = 2 * time.Second

// newActiveGraphKeyChecker answers "is there already an active incident on
// this graph node?" from a cached index.
//
// The direct form walked every active incident for every dependency of every
// diagnosis. On a cluster holding a few hundred incidents that is tens of
// thousands of string splits per notification, on the delivery path.
func newActiveGraphKeyChecker(
	engine *correlation.Engine,
	now func() time.Time,
) func(kind, namespace, name string) bool {
	var (
		mu      sync.Mutex
		keys    map[string]bool
		expires time.Time
	)
	return func(kind, namespace, name string) bool {
		mu.Lock()
		defer mu.Unlock()
		if keys == nil || now().After(expires) {
			keys = make(map[string]bool)
			for _, inc := range engine.ActiveIncidents() {
				for _, key := range insight.IncidentGraphKeys(inc) {
					keys[key] = true
				}
			}
			expires = now().Add(activeKeyTTL)
		}
		return keys[kind+"/"+namespace+"/"+name]
	}
}
