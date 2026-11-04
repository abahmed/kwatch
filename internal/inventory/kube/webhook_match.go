package kube

import (
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/labels"

	"github.com/abahmed/kwatch/internal/inventory"
)

// WebhookReach is what the fail-closed webhooks that call one Service
// intercept among the objects that exist now, as far as the model can
// tell (AttrWebhookClosedRules).
type WebhookReach struct {
	// Listed is false when the configuration lists no rule for the
	// Service: what its webhooks intercept is not known.
	Listed bool
	// Resources are the resources ("pods") with at least one object
	// that a create or update rule intercepts now, sorted.
	Resources []string
	// Unknown is set when a rule could not be judged: a resource the
	// model does not hold, a kind not synced, a selector it cannot
	// evaluate, or a rule for operations other than create and update.
	Unknown bool
}

// None reports rules that are all known to intercept no object that
// exists now, such as rules for custom resources whose definition is
// gone.
func (r WebhookReach) None() bool {
	return r.Listed && !r.Unknown && len(r.Resources) == 0
}

// ClosedRuleReach judges the rules of the fail-closed webhooks of config
// that call service against the model. synced reports kinds whose
// source holds the whole list.
func ClosedRuleReach(
	model inventory.Reader, synced func(inventory.Kind) bool,
	config inventory.Entity, service inventory.EntityID,
) WebhookReach {
	var reach WebhookReach
	key := service.Namespace + "/" + service.Name
	text := textOf(config, AttrWebhookClosedRules)
	seen := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Split(line, "|")
		if len(fields) != closedRuleFields || fields[0] != key {
			continue
		}
		reach.Listed = true
		found, known := ruleReach(model, synced, fields[1:])
		reach.Unknown = reach.Unknown || !known
		for _, resource := range found {
			if !seen[resource] {
				seen[resource] = true
				reach.Resources = append(reach.Resources, resource)
			}
		}
	}
	sort.Strings(reach.Resources)
	return reach
}

// ruleReach judges one rule line (group, resource, scope, writes,
// namespace selector, object selector): the resources it intercepts
// now, and whether that is known.
func ruleReach(
	model inventory.Reader, synced func(inventory.Kind) bool,
	f []string,
) ([]string, bool) {
	group, resource := f[0], f[1]
	wildcard := resource == "*" || resource == "*/*"
	if f[3] != "1" || (!wildcard && strings.Contains(resource, "/")) {
		return nil, false
	}
	targets := typedTargets(group, resource, wildcard)
	if len(targets) == 0 {
		return nil, customResourceGone(model, synced, group, resource,
			wildcard)
	}
	match, err := newRuleMatch(f[2], f[4], f[5])
	if err != nil {
		return nil, false
	}
	var found []string
	known := true
	for _, reg := range targets {
		kind := reg.schema.Kind()
		if !synced(kind) {
			known = false
			continue
		}
		hit, ok := match.anyIn(model, synced, kind)
		known = known && ok
		if hit {
			found = append(found, reg.resource.Name)
		}
	}
	return found, known
}

// typedTargets are the typed resources a rule's group and resource
// name.
func typedTargets(group, resource string, wildcard bool) []registration {
	var out []registration
	for _, reg := range registrations() {
		if (group == "*" || group == reg.resource.Group) &&
			(wildcard || resource == reg.resource.Name) {
			out = append(out, reg)
		}
	}
	return out
}

// customResourceGone reports a rule for a custom resource whose
// definition the model knows is gone: nothing of it exists. Any other
// resource the model does not hold is unknown.
func customResourceGone(
	model inventory.Reader, synced func(inventory.Kind) bool,
	group, resource string, wildcard bool,
) bool {
	if wildcard || group == "*" || GroupFor(group) == "" ||
		!synced(KindCRD) {
		return false
	}
	return !model.Exists(inventory.CoreID(KindCRD, "",
		resource+"."+group))
}

// ruleMatch is the scope and selectors of one rule. namespaces caches
// the labels of the namespaces looked at, so a scan of many pods reads
// each namespace once.
type ruleMatch struct {
	scope            string
	namespace, label labels.Selector
	namespaces       map[inventory.EntityID]labelsRead
}

// labelsRead is an object's labels and whether the model holds them.
type labelsRead struct {
	set labels.Set
	ok  bool
}

func newRuleMatch(scope, nsSel, objSel string) (ruleMatch, error) {
	m := ruleMatch{scope: scope,
		namespaces: map[inventory.EntityID]labelsRead{}}
	var err error
	if nsSel != "" {
		if m.namespace, err = labels.Parse(nsSel); err != nil {
			return m, err
		}
	}
	if objSel != "" {
		m.label, err = labels.Parse(objSel)
	}
	return m, err
}

// anyIn reports whether an object of kind is intercepted, and whether
// the model could tell for every object it looked at.
func (m ruleMatch) anyIn(
	model inventory.Reader, synced func(inventory.Kind) bool,
	kind inventory.Kind,
) (bool, bool) {
	known := true
	for _, id := range model.Entities(kind) {
		if !m.inScope(id) {
			continue
		}
		hit, ok := m.selects(model, synced, id)
		if hit {
			return true, true
		}
		known = known && ok
	}
	return false, known
}

// inScope applies the rule's scope: Cluster for cluster-scoped objects,
// Namespaced for namespaced ones, "*" for both.
func (m ruleMatch) inScope(id inventory.EntityID) bool {
	switch m.scope {
	case "Cluster":
		return id.Namespace == ""
	case "Namespaced":
		return id.Namespace != ""
	}
	return true
}

// selects applies the selectors to one object: the namespace selector
// to its namespace's labels, the object selector to its labels. It
// reports a match and whether the model could tell.
func (m ruleMatch) selects(
	model inventory.Reader, synced func(inventory.Kind) bool,
	id inventory.EntityID,
) (bool, bool) {
	if hit, ok := m.namespaceSelects(model, synced, id); !hit || !ok {
		return false, ok
	}
	if m.label == nil {
		return true, true
	}
	set, ok := labelsOf(model, id)
	if !ok {
		return false, false
	}
	return m.label.Matches(set), true
}

// namespaceSelects applies the namespace selector: to the namespace's
// own labels for a Namespace, to its namespace's labels for a namespaced
// object; a cluster-scoped object is never skipped by it.
func (m ruleMatch) namespaceSelects(
	model inventory.Reader, synced func(inventory.Kind) bool,
	id inventory.EntityID,
) (bool, bool) {
	if m.namespace == nil ||
		(id.Namespace == "" && id.Kind != KindNamespace) {
		return true, true
	}
	ns := id
	if id.Kind != KindNamespace {
		ns = inventory.CoreID(KindNamespace, "", id.Namespace)
	}
	read, cached := m.namespaces[ns]
	if !cached {
		read.set, read.ok = labelsOf(model, ns)
		m.namespaces[ns] = read
	}
	if !read.ok || !synced(KindNamespace) {
		return false, false
	}
	return m.namespace.Matches(read.set), true
}

// labelsOf is an object's labels when the model holds them.
func labelsOf(
	model inventory.Reader, id inventory.EntityID,
) (labels.Set, bool) {
	e, ok := model.Entity(id)
	if !ok {
		return nil, false
	}
	attr, ok := e.Attribute(AttrLabels)
	if !ok {
		return nil, false
	}
	text := attr.Value.AsText()
	if text == "" {
		return labels.Set{}, true
	}
	return labels.Set(ParseLabels(text)), true
}
