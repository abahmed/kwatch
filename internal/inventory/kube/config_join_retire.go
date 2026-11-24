package kube

import (
	"sort"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// WithModel lets the join read the model, to tell whether another source
// still describes an endpoint it stops naming. Without it the join takes
// itself for the endpoint's only source.
func (j *ConfigJoin) WithModel(model inventory.Reader) *ConfigJoin {
	j.model = model
	return j
}

// withdrawEndpoint is what the join says of an endpoint no ConfigMap a
// pod reads names any more. When another source still describes it (an
// active prober probes it, and retires it once nobody calls it) the
// join takes back only its own attribute. Otherwise the join made the
// endpoint and nothing else will remove it: it is gone.
func (j *ConfigJoin) withdrawEndpoint(
	endpoint string, at time.Time,
) inventory.Observation {
	id := endpointOf(endpoint)
	if j.describedByOthers(id) {
		j.withdrawn[endpoint] = true
		return inventory.Observation{
			Kind: inventory.Observed, Source: ConfigSource, At: at,
			Entity: id,
		}
	}
	return inventory.Observation{
		Kind: inventory.Gone, Source: ConfigSource, At: at, Entity: id,
	}
}

// recheckWithdrawn decides again, on every pass, for the endpoints the
// join withdrew from while another source described them. That answer
// was read from the model at one moment, and the other source may have
// let go since (its last observation raced the join's): an endpoint
// nobody describes any more is removed now, so it is not left behind
// with no source to remove it. One a ConfigMap names again is the
// join's once more.
func (j *ConfigJoin) recheckWithdrawn(
	at time.Time,
) []inventory.Observation {
	endpoints := make([]string, 0, len(j.withdrawn))
	for endpoint := range j.withdrawn {
		endpoints = append(endpoints, endpoint)
	}
	sort.Strings(endpoints)
	var out []inventory.Observation
	for _, endpoint := range endpoints {
		id := endpointOf(endpoint)
		if _, named := j.said[endpoint]; named {
			delete(j.withdrawn, endpoint)
			continue
		}
		if j.describedByOthers(id) {
			continue
		}
		delete(j.withdrawn, endpoint)
		if j.model.Exists(id) {
			out = append(out, inventory.Observation{
				Kind: inventory.Gone, Source: ConfigSource, At: at,
				Entity: id,
			})
		}
	}
	return out
}

// describedByOthers reports whether the endpoint carries an attribute
// the join did not write.
func (j *ConfigJoin) describedByOthers(id inventory.EntityID) bool {
	if j.model == nil {
		return false
	}
	entity, ok := j.model.Entity(id)
	if !ok {
		return false
	}
	for name := range entity.Attributes {
		if name != AttrEndpointSources {
			return true
		}
	}
	return false
}
