package dashboard

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// InsightQuiet explains a cluster with no active issue: how much
// kwatch watches and when it last looked. A count is absent when the
// source behind it is not running.
type InsightQuiet struct {
	Objects    *int      `json:"objects,omitempty"`
	Namespaces *int      `json:"namespaces,omitempty"`
	Kinds      *int      `json:"kinds,omitempty"`
	LastScan   time.Time `json:"lastScan,omitzero"`
}

// modelStats is what the live inventory says about its size;
// *inventory.Model provides it.
type modelStats interface {
	Stats() inventory.Stats
}

// explainQuiet fills q from the inventory, the coverage source and the
// engine's latest scan.
func (s *Server) explainQuiet(q *InsightQuiet, model inventory.Reader) {
	if st, ok := model.(modelStats); ok {
		n := st.Stats().Entities
		q.Objects = &n
	}
	if src := s.sources.coverage.Load(); src != nil {
		c := (*src)()
		ns, kinds := c.Namespaces.Watched, c.Kinds.Watched
		q.Namespaces, q.Kinds = &ns, &kinds
	}
	if at := s.sources.lastScan.Load(); at != 0 {
		q.LastScan = time.Unix(0, at).UTC()
	}
}
