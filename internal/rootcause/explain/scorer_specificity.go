package explain

import (
	"fmt"
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// scoreSpecificity rewards a cause the error text names: "couldn't
// find key db-password in Secret shop/db-creds" names the secret. At
// least half of the effects must name it.
func scoreSpecificity(v *view, c *candidate) outcome {
	effects := c.others()
	// Pods are named after their owners, so an owner is always
	// "named"; that is not evidence.
	if len(effects) == 0 || c.isSelf() || c.bestMatch().link == LinkOwns {
		return outcome{}
	}
	names := v.namesOf(c.id)
	if len(names) == 0 {
		return outcome{}
	}
	named := 0
	for _, effect := range effects {
		if mentionsAny(v.errorText(effect), names) {
			named++
		}
	}
	if named*2 < len(effects) {
		return outcome{}
	}
	return outcome{weight: SpecificityWeight,
		code: rootcause.ProofErrorsName, count: named, total: len(effects),
		text: fmt.Sprintf(
			"the errors of %d of %d failures name it", named, len(effects))}
}

// namesOf lists the words that identify id in error text: its name and
// the names of the Services that serve it, since a webhook error names
// the webhook's Service rather than its configuration.
func (v *view) namesOf(id inventory.EntityID) []string {
	var out []string
	add := func(name string) {
		if len(name) >= SpecificityMinName {
			out = append(out, strings.ToLower(name))
		}
	}
	add(id.Name)
	for _, service := range v.s.Model.Related(
		id, inventory.Serves, inventory.Outgoing,
	) {
		add(service.Name)
	}
	return out
}

// mentionsAny reports whether text contains one of the lower-case
// names.
func mentionsAny(text string, names []string) bool {
	text = strings.ToLower(text)
	for _, name := range names {
		if strings.Contains(text, name) {
			return true
		}
	}
	return false
}
