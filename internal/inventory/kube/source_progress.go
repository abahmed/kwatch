package kube

// SyncProgress is how far the first sync of the typed watches is, for a
// "starting" view while kwatch lists the cluster.
type SyncProgress struct {
	// Kinds counts the typed watches; Synced those whose initial list
	// is in the model; Settled those no longer holding up startup
	// (synced, or refused for good: RBAC, a type the API does not
	// serve).
	Kinds   int `json:"kinds"`
	Synced  int `json:"synced"`
	Settled int `json:"settled"`
	// Required and RequiredSynced count the watches startup waits for
	// before the first alert (WaitForRequiredSync).
	Required       int `json:"required"`
	RequiredSynced int `json:"requiredSynced"`
	// Objects counts the objects of initial lists handed to the model
	// so far. It only grows: relists after startup add to it too.
	Objects int64 `json:"objects"`
	// Done is set once WaitForSync saw every kind settle (SyncedAt).
	Done bool `json:"done"`
}

// SyncProgress reports the first sync's progress. It is safe for
// concurrent use and cheap enough to poll from a status page.
func (s *Source) SyncProgress() SyncProgress {
	out := SyncProgress{
		Kinds: len(s.watches), Objects: s.listed.Load(),
		Done: s.syncedAt.Load() != 0,
	}
	for _, w := range s.watches {
		synced := w.hasSynced()
		if synced {
			out.Synced++
		}
		if w.settled() {
			out.Settled++
		}
		if w.required {
			out.Required++
			if synced {
				out.RequiredSynced++
			}
		}
	}
	return out
}
