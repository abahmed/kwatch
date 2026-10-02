package explain

import (
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// scoreChange rewards a candidate changed inside the causal window,
// no later than the failures began. A replica-count change alone is
// not evidence: scaling adds pods, it does not break them. A change
// the row itself matched is not counted twice.
func scoreChange(v *view, c *candidate) outcome {
	// A row that matched the change itself already counts it.
	if c.bestMatch().match.causeMode.Within(ModeChanged) {
		return outcome{}
	}
	first := v.earliestSince(c.others())
	for _, change := range v.changesOf(c.id) {
		if changeMode(change) == ModeScaled {
			continue
		}
		if first.ok && change.At.After(first.t.Add(TemporalSlack)) {
			continue
		}
		return changeOutcome(change)
	}
	return outcome{}
}

// changeOutcome is the evidence of a change shortly before the
// failures: a creation, or the fields it touched when they are known.
func changeOutcome(change inventory.Change) outcome {
	out := outcome{weight: ChangeWeight, code: rootcause.ProofChanged,
		text: "it changed shortly before: " + describeChange(change)}
	if change.Created {
		out.code = rootcause.ProofCreated
		return out
	}
	for _, field := range change.Fields {
		out.fields = append(out.fields, field.Path)
	}
	return out
}

// describeChange names what a change touched, as "data.db-password".
func describeChange(change inventory.Change) string {
	if change.Created {
		return "created"
	}
	var paths []string
	for _, field := range change.Fields {
		paths = append(paths, field.Path)
	}
	if len(paths) == 0 {
		return "changed"
	}
	return strings.Join(paths, ", ")
}
