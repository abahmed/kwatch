package explain

import (
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// admissionCheck is one kind of object that can refuse the creates of a
// controller: a webhook, a quota, a LimitRange, a namespace's Pod
// Security label, a validating policy. Every "creation rejected by X"
// question goes through this table: which objects may be rejecting (hops),
// what state the effect's own error text shows for one (modes), and
// whether that text rules it out (ruledOut).
type admissionCheck struct {
	// hops lead from a controller that cannot create objects to the
	// objects of this kind that may be rejecting the creates.
	hops func(v *view, controller inventory.EntityID) []hop
	// modes are the pseudo modes of an object of this kind, read from
	// the effect's error. Nil when the kind has none.
	modes func(
		v *view, id, effect inventory.EntityID, link LinkType,
	) []modeHealth
	// exclusive makes modes the only source of the kind's pseudo
	// modes, even when it finds none.
	exclusive bool
	// ruledOut reports that the lower-cased error names something else
	// as the refuser. Nil when the text never rules the kind out.
	ruledOut func(v *view, id inventory.EntityID, text string) bool
}

// admissionChecks lists the checks in the order their hops are tried.
// Each kind that can refuse a create is listed under its entity kind.
var admissionChecks = []struct {
	kinds []inventory.Kind
	check admissionCheck
}{
	{admitKinds, admissionCheck{
		hops:      (*view).webhookAdmitHops,
		modes:     (*view).admissionModes,
		exclusive: true,
		ruledOut:  (*view).otherWebhook,
	}},
	{[]inventory.Kind{kube.KindQuota}, admissionCheck{
		hops:     (*view).quotaAdmitHops,
		ruledOut: (*view).otherQuota,
	}},
	{[]inventory.Kind{kube.KindNamespace}, admissionCheck{
		modes: (*view).podSecurityModes,
	}},
	{[]inventory.Kind{kube.KindLimitRange}, admissionCheck{
		modes: (*view).limitRangeModes,
	}},
	{[]inventory.Kind{kindAdmissionPolicy}, admissionCheck{
		hops:  (*view).policyAdmitHops,
		modes: (*view).policyModes,
	}},
	{[]inventory.Kind{kube.KindAccount}, admissionCheck{
		hops: (*view).accountAdmitHops,
	}},
}

// admissionCheckFor is the check of an entity kind, if it can refuse
// creates.
func admissionCheckFor(kind inventory.Kind) (admissionCheck, bool) {
	for _, entry := range admissionChecks {
		for _, k := range entry.kinds {
			if k == kind {
				return entry.check, true
			}
		}
	}
	return admissionCheck{}, false
}

// allAdmitHops lead from a controller that cannot create objects to
// everything that may be rejecting the creates.
func (v *view) allAdmitHops(controller inventory.EntityID) []hop {
	var out []hop
	for _, entry := range admissionChecks {
		if entry.check.hops != nil {
			out = append(out, entry.check.hops(v, controller)...)
		}
	}
	return out
}

// refusalModes are the pseudo modes of an object that may have refused
// the effect's creates; ok is false for kinds that never do.
func (v *view) refusalModes(
	id, effect inventory.EntityID, link LinkType,
) (modes []modeHealth, ok bool) {
	check, found := admissionCheckFor(id.Kind)
	if !found || check.modes == nil {
		return nil, false
	}
	modes = check.modes(v, id, effect, link)
	return modes, len(modes) > 0 || check.exclusive
}

// admitsElsewhere reports whether an admission candidate is ruled out
// by the effect's own words: the error names another webhook or quota,
// or the webhook fails open, so its failed call rejected nothing.
func (v *view) admitsElsewhere(id, effect inventory.EntityID) bool {
	check, ok := admissionCheckFor(id.Kind)
	if !ok || check.ruledOut == nil {
		return false
	}
	return check.ruledOut(v, id, strings.ToLower(v.text(effect)))
}
