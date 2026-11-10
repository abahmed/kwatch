package dashboard

import (
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/redact"
)

// LeftoverObject is one object a removed or stopped controller left
// behind, in an issue that groups them: what it is and, in one short
// line, what is wrong with it. An object with several failures is one
// row that names them all, so the rows count objects, as
// leftoverObjects does.
type LeftoverObject struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
	Reason    string `json:"reason"`
	What      string `json:"what"`
}

// maxLeftovers bounds the objects a detail lists; the list row's
// leftoverObjects says how many there are.
const maxLeftovers = 100

// toLeftovers lists the objects of a leftover issue, one row each,
// redacted; nil for any other issue.
func toLeftovers(p incident.Incident) []LeftoverObject {
	members, ok := p.Leftovers()
	if !ok {
		return nil
	}
	out := make([]LeftoverObject, 0, min(len(members), maxLeftovers))
	var last inventory.EntityID
	for _, m := range members {
		what := redact.Evidence(m.Summary)
		if n := len(out); n > 0 && m.Entity == last {
			out[n-1].Reason = joinNew(out[n-1].Reason, m.Reason, ", ")
			out[n-1].What = joinNew(out[n-1].What, what, "; ")
			continue
		}
		if len(out) == maxLeftovers {
			break
		}
		last = m.Entity
		out = append(out, LeftoverObject{
			Kind:      kindDisplay(m.Entity),
			Namespace: m.Entity.Namespace, Name: m.Entity.Name,
			Reason: m.Reason, What: what})
	}
	return out
}

// joinNew adds more to have behind sep, unless it is empty or already
// the whole of it.
func joinNew(have, more, sep string) string {
	if more == "" || more == have {
		return have
	}
	if have == "" {
		return more
	}
	return have + sep + more
}
