package dashboard

import (
	"fmt"

	"github.com/abahmed/kwatch/internal/detection/reasons"
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

// leftoverWhat is the summary of an issue that groups what a removed or
// stopped controller left behind: "Leftovers of Deployment op in ops:
// 4 objects it left behind while it has no ready pods." The root is
// not stuck deleting nor expired: its members are, each in the list.
// ok is false for any other issue, and for a lone leftover.
func leftoverWhat(p incident.Incident) (string, bool) {
	n := p.LeftoverObjects()
	if n < 2 || p.Root.Name == "" {
		return "", false
	}
	now, was := "is gone or has nothing ready",
		"was gone or had nothing ready"
	for _, m := range p.Members {
		switch m.Reason {
		case reasons.WebhookBackendNotFound:
			now, was = "is gone", "was gone"
		case reasons.WebhookNoEndpoints:
			if now != "is gone" {
				now, was = "has no ready pods", "had no ready pods"
			}
		}
	}
	state := now
	if p.State == incident.Resolved {
		state = was
	}
	return fmt.Sprintf("Leftovers of %s: %d objects it left behind "+
		"while it %s.", rootWords(p.Root), n, state), true
}
