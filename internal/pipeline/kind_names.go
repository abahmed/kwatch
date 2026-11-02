package pipeline

import (
	"maps"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// withKindNames gives d the API's spelling of every custom kind among
// the entities its message may name, read from their kind.name
// attribute. The writer reads them as data, never the model. The map
// d arrived with may be shared with the incident's stored facts, so it
// is copied, never written.
func (a *announcer) withKindNames(d incident.Decision) incident.Decision {
	names := a.kindNames(d.Incident)
	if len(names) == 0 {
		return d
	}
	merged := maps.Clone(d.Facts.KindNames)
	if merged == nil {
		merged = make(map[inventory.Kind]string, len(names))
	}
	maps.Copy(merged, names)
	d.Facts.KindNames = merged
	return d
}

// kindNames is the API's spelling of every custom kind among the
// entities a message about p may name; nil when none is known.
func (a *announcer) kindNames(p incident.Incident) map[inventory.Kind]string {
	if a.history == nil {
		return nil
	}
	ids := []inventory.EntityID{p.Root}
	ids = append(ids, p.Impact...)
	for key := range p.Members {
		ids = append(ids, key.Entity)
	}
	if cause := p.Cause; cause != nil {
		ids = append(ids, cause.Root)
		ids = append(ids, cause.Chain...)
	}
	var out map[inventory.Kind]string
	for _, id := range ids {
		entity, ok := a.history.Entity(id)
		if !ok {
			continue
		}
		attr, ok := entity.Attribute(kube.AttrKindName)
		if !ok {
			continue
		}
		if out == nil {
			out = map[inventory.Kind]string{}
		}
		out[id.Kind] = attr.Value.AsText()
	}
	return out
}
