package kube

import (
	"sort"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// KindDeprecatedAPI is a virtual entity for one deprecated API version
// that something requested since the API server started, for example
// policy/v1beta1 poddisruptionbudgets. The API server reports it in
// apiserver_requested_deprecated_apis.
const KindDeprecatedAPI inventory.Kind = "deprecated-api"

// Attributes of a KindDeprecatedAPI entity.
const (
	AttrDeprecatedGroup    = "deprecated.api.group"
	AttrDeprecatedVersion  = "deprecated.api.version"
	AttrDeprecatedResource = "deprecated.api.resource"
	// AttrDeprecatedRemoved is the Kubernetes release that removes the
	// API, such as "1.25".
	AttrDeprecatedRemoved = "deprecated.api.removed"
)

// deprecatedAPIMetric is the gauge the API server sets to 1 for every
// deprecated API that was requested since it started.
const deprecatedAPIMetric = "apiserver_requested_deprecated_apis"

// DeprecatedAPI is one series of that gauge.
type DeprecatedAPI struct {
	Group, Version string
	// Resource includes the subresource: "pods/eviction".
	Resource string
	// Removed is the release that removes it. It is empty when the API
	// is deprecated but no removal is planned.
	Removed string
}

// ID is the entity of the API. Its name is how a person says it:
// "policy/v1beta1 poddisruptionbudgets", or "v1 componentstatuses" in
// the core group.
func (d DeprecatedAPI) ID() inventory.EntityID {
	name := d.Version + " " + d.Resource
	if d.Group != "" {
		name = d.Group + "/" + name
	}
	return inventory.CoreID(KindDeprecatedAPI, "", name)
}

// deprecatedAPIs reads the deprecated APIs a /metrics response lists,
// sorted by entity. A series with value 0 is not a request.
func deprecatedAPIs(body []byte) []DeprecatedAPI {
	found := map[inventory.EntityID]DeprecatedAPI{}
	forEachMetric(body, deprecatedAPIMetric, func(
		labels map[string]string, value float64,
	) {
		if value < 1 || labels["version"] == "" || labels["resource"] == "" {
			return
		}
		api := DeprecatedAPI{
			Group: labels["group"], Version: labels["version"],
			Resource: labels["resource"], Removed: labels["removed_release"],
		}
		if sub := labels["subresource"]; sub != "" {
			api.Resource += "/" + sub
		}
		found[api.ID()] = api
	})
	out := make([]DeprecatedAPI, 0, len(found))
	for _, api := range found {
		out = append(out, api)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID().Name < out[j].ID().Name
	})
	return out
}

// deprecatedAPIObservations records the deprecated APIs of one response
// that a release will remove. APIs with no planned removal (such as
// core/v1 endpoints) give nothing to act on and are left out.
//
// The gauge belongs to one API server process, and the request goes
// through the kubernetes Service, so another process may answer the next
// round without the series. An API is therefore kept until it has been
// missing for metricsKeepFor; the gauge only resets when the API server
// restarts, so a missing series is the end of the problem, not a blip.
func (p *Prober) deprecatedAPIObservations(
	body []byte, now time.Time,
) []inventory.Observation {
	if p.deprecatedSeen == nil {
		p.deprecatedSeen = map[inventory.EntityID]time.Time{}
	}
	var out []inventory.Observation
	for _, api := range deprecatedAPIs(body) {
		if api.Removed == "" {
			continue
		}
		p.deprecatedSeen[api.ID()] = now
		out = append(out, inventory.Observation{
			Kind: inventory.Observed, Source: ProbeSource, At: now,
			Entity: api.ID(),
			Attributes: map[string]inventory.Value{
				AttrDeprecatedGroup:    inventory.Text(api.Group),
				AttrDeprecatedVersion:  inventory.Text(api.Version),
				AttrDeprecatedResource: inventory.Text(api.Resource),
				AttrDeprecatedRemoved:  inventory.Text(api.Removed),
			},
		})
	}
	for id, last := range p.deprecatedSeen {
		if now.Sub(last) > metricsKeepFor {
			delete(p.deprecatedSeen, id)
			out = append(out, inventory.Observation{
				Kind: inventory.Gone, Source: ProbeSource, At: now,
				Entity: id,
			})
		}
	}
	return out
}
