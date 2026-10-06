package pipeline

import (
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// withKindNames gives d the API's spelling of every custom kind among
// the entities its message may name, read from their kind.name
// attribute. The writer reads them as data, never the model.
func (a *announcer) withKindNames(d incident.Decision) incident.Decision {
	if a.history == nil {
		return d
	}
	ids := []inventory.EntityID{d.Incident.Root}
	ids = append(ids, d.Incident.Impact...)
	for key := range d.Incident.Members {
		ids = append(ids, key.Entity)
	}
	if cause := d.Incident.Cause; cause != nil {
		ids = append(ids, cause.Root)
		ids = append(ids, cause.Chain...)
	}
	for _, id := range ids {
		entity, ok := a.history.Entity(id)
		if !ok {
			continue
		}
		attr, ok := entity.Attribute(kube.AttrKindName)
		if !ok {
			continue
		}
		if d.Facts.KindNames == nil {
			d.Facts.KindNames = map[inventory.Kind]string{}
		}
		d.Facts.KindNames[id.Kind] = attr.Value.AsText()
	}
	return d
}
