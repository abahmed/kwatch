package incident

import (
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// CauseState says what looking at an incident's stated cause again, at
// the moment it resolves, found.
type CauseState uint8

// Cause states.
const (
	// CauseUnchecked: the cause was not looked at, or its kind has no
	// check. The resolve says nothing about it.
	CauseUnchecked CauseState = iota
	// CauseRecovered: the cause was seen working again.
	CauseRecovered
	// CauseStill: the cause still shows the state it was blamed for.
	CauseStill
)

// CheckCause looks at the incident's stated cause once more. The checks
// are the plain facts of three kinds of cause: a node that is Ready, an
// external endpoint whose probe is ok, and a Service with ready
// endpoints. Any other cause, and a cause the model no longer knows,
// is CauseUnchecked.
func CheckCause(
	model inventory.Reader, cause *rootcause.CauseRecord,
) CauseState {
	if model == nil || cause == nil {
		return CauseUnchecked
	}
	e, ok := model.Entity(cause.Root)
	if !ok {
		return CauseUnchecked
	}
	var working, known bool
	switch cause.Root.Kind {
	case kube.KindNode:
		working, known = boolAttr(e, kube.AttrReady)
	case kube.KindExternalEndpoint:
		working, known = boolAttr(e, kube.AttrHealthy)
	case kube.KindService:
		working, known = serviceServes(model, e)
	}
	switch {
	case !known:
		return CauseUnchecked
	case working:
		return CauseRecovered
	}
	return CauseStill
}

func boolAttr(e inventory.Entity, name string) (value, known bool) {
	attribute, ok := e.Attribute(name)
	if !ok {
		return false, false
	}
	value, known = attribute.Value.AsBool()
	return value, known
}

// serviceServes reports whether the Service has a ready endpoint.
func serviceServes(
	model inventory.Reader, e inventory.Entity,
) (serves, known bool) {
	slices := model.Related(e.ID, inventory.Backs, inventory.Incoming)
	if len(slices) == 0 {
		return false, false
	}
	for _, id := range slices {
		slice, ok := model.Entity(id)
		if !ok {
			continue
		}
		if attribute, ok := slice.Attribute(kube.AttrEndpointsReady); ok {
			if n, _ := attribute.Value.AsNumber(); n > 0 {
				return true, true
			}
		}
	}
	return false, true
}
