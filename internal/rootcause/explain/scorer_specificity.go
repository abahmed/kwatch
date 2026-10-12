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
	named := 0
	for _, effect := range effects {
		if mentionsAny(v.errorText(effect), names) ||
			v.errorNamesBlockedCall(c, effect) {
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

// namesOf lists the words that identify id in error text: its name, the
// keys that changed in it and the names of the Services that serve it,
// since a webhook error names the webhook's Service rather than its
// configuration.
func (v *view) namesOf(id inventory.EntityID) []string {
	var out []string
	add := func(name string) {
		if len(name) >= SpecificityMinName {
			out = append(out, strings.ToLower(name))
		}
	}
	add(id.Name)
	for _, key := range v.changedKeys(id) {
		add(key)
	}
	for _, service := range v.s.Model.Related(
		id, inventory.Serves, inventory.Outgoing,
	) {
		add(service.Name)
	}
	return out
}

// mentionsAny reports whether text names one of the lower-case names.
func mentionsAny(text string, names []string) bool {
	text = strings.ToLower(text)
	for _, name := range names {
		if mentionsWord(text, name) {
			return true
		}
	}
	return false
}

// mentionsWord reports name as a whole word of text: not part of a
// longer name. Object names are made of letters, digits and dashes, so
// "api" is not named by "rapid" or "api-gateway" but is by `"api"`,
// "secret api" and "api.example.com".
func mentionsWord(text, name string) bool {
	for from := 0; from < len(text); {
		at := strings.Index(text[from:], name)
		if at < 0 {
			return false
		}
		start := from + at
		end := start + len(name)
		if (start == 0 || !nameByte(text[start-1])) &&
			(end == len(text) || !nameByte(text[end])) {
			return true
		}
		from = start + 1
	}
	return false
}

// nameByte reports a byte that continues an object or key name.
func nameByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= '0' && b <= '9' ||
		b == '-' || b == '_'
}

// changedKeys lists the keys of id's ConfigMap or Secret that changed
// ("data.db-password"): an error that quotes one points at the change.
func (v *view) changedKeys(id inventory.EntityID) []string {
	var out []string
	for _, change := range v.changesOf(id) {
		for _, field := range change.Fields {
			if key, ok := strings.CutPrefix(field.Path, "data."); ok {
				out = append(out, key)
			}
		}
	}
	return out
}

// errorNamesBlockedCall reports a policy that stops exactly the call
// the effect's own error names (an address or a Service it dialed): a
// call read from configuration alone proves nothing about the error.
func (v *view) errorNamesBlockedCall(
	c *candidate, effect inventory.EntityID,
) bool {
	if c.covers[effect].match.row.Name != "policy-blocks-call" {
		return false
	}
	pod, ok := v.unitOf(effect)
	if !ok {
		return false
	}
	edge, blocked := v.blocks[blockKey{c.id, pod}]
	call, named := v.clusterCallOf(pod)
	return blocked && named && !call.cascade && call.service == edge.service
}
