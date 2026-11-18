package explain

import (
	"regexp"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
)

// Some clients say what kind of service they could not reach but not
// where ("It was not possible to connect to the redis server(s)"). The
// kind is read from two small tables of data, so a new client is a new
// row, never new logic. Text the tables do not know keeps today's
// behaviour: it groups by its normalised error line.

// techKinds name the kind of service a client name (clientNames) is.
var techKinds = map[string]string{
	"Redis": "cache", "Memcached": "cache",
	"PostgreSQL": "database", "MySQL": "database", "MongoDB": "database",
	"Cassandra": "database", "SQL Server": "database",
	"Kafka": "message queue", "RabbitMQ": "message queue",
	"Elasticsearch": "search service",
}

// kindWords name the kind when the error names no client, only the kind
// itself ("could not connect to the database"), first match wins.
var kindWords = []struct {
	kind    string
	pattern *regexp.Regexp
}{
	{"database", regexp.MustCompile(`\bdatabase\b|\bdb server`)},
	{"cache", regexp.MustCompile(`\bcache server|\bcache\b`)},
	{"message queue", regexp.MustCompile(`\bmessage (?:queue|broker)|` +
		`\bbrokers?\b`)},
}

// DependencyKind is the kind of service a failed connection was to,
// with the technology when the error itself states it.
type DependencyKind struct {
	Kind, Tech string
}

// Label is "cache (Redis)", or the kind alone.
func (k DependencyKind) Label() string {
	if k.Tech == "" {
		return k.Kind
	}
	return k.Kind + " (" + k.Tech + ")"
}

// dependencyKindIn reads the kind of service a connection failure names,
// with or without an address.
func dependencyKindIn(text string) (DependencyKind, bool) {
	lower := strings.ToLower(capText(text, dependencyScanMax))
	if !connectionFailure.MatchString(lower) {
		return DependencyKind{}, false
	}
	d, _ := DependencyIn(text)
	if kind, ok := techKinds[d.Name]; ok {
		return DependencyKind{Kind: kind, Tech: d.Name}, true
	}
	for _, row := range kindWords {
		if row.pattern.MatchString(lower) {
			return DependencyKind{Kind: row.kind}, true
		}
	}
	return DependencyKind{}, false
}

// KindOnlyDependency is the kind of service a connection failure was
// to, when the error names no address: neither a host nor a URL.
func KindOnlyDependency(text string) (DependencyKind, bool) {
	if _, named := endpointIn(text); named {
		return DependencyKind{}, false
	}
	if d, _ := DependencyIn(text); d.Host != "" {
		return DependencyKind{}, false
	}
	return dependencyKindIn(text)
}

// kindOnlyOf is the kind of service the failing pod's own words say it
// cannot reach, when they name no address.
func (v *view) kindOnlyOf(pod inventory.EntityID) (DependencyKind, bool) {
	if _, named := v.callOf(pod); named {
		return DependencyKind{}, false
	}
	unit, ok := v.unitOf(pod)
	if !ok {
		return DependencyKind{}, false
	}
	for _, f := range v.unitFindingsIn(unit, unreadyCallers) {
		for _, text := range v.callTexts(f) {
			if k, ok := KindOnlyDependency(text); ok {
				return k, true
			}
		}
	}
	return DependencyKind{}, false
}

// kindOnlyEndpoint is the one endpoint the pod's kind-only failure is
// the same failure of: other failing pods name it, their errors say it
// is the same kind of service, and they began failing within
// SignatureWindow of the pod. Two endpoints of that kind failing, or
// none, leave the pod to its error line.
func (v *view) kindOnlyEndpoint(pod inventory.EntityID) (string, bool) {
	kind, ok := v.kindOnlyOf(pod)
	if !ok {
		return "", false
	}
	unit, _ := v.unitOf(pod)
	start := v.earliestSince(append(v.s.Model.Related(unit,
		inventory.PartOf, inventory.Incoming), unit))
	found := ""
	for endpoint, began := range v.endpointsOfKind(kind.Kind) {
		if start.ok && began.ok && absDuration(start.t.Sub(began.t)) >
			SignatureWindow {
			continue
		}
		if found != "" {
			return "", false
		}
		found = endpoint
	}
	return found, found != ""
}

// endpointsOfKind maps each endpoint the failing pods name, whose
// errors say it is a service of kind, to when its callers began failing.
func (v *view) endpointsOfKind(kind string) map[string]timeValue {
	if v.kindEndpoints == nil {
		v.kindEndpoints = map[string]map[string]timeValue{}
		for _, id := range sortedFindingIDs(v.s) {
			v.indexKindEndpoint(id)
		}
	}
	return v.kindEndpoints[kind]
}

// indexKindEndpoint adds the endpoint id's unit names, by its kind.
func (v *view) indexKindEndpoint(id inventory.EntityID) {
	call, ok := v.callOf(id)
	if !ok {
		return
	}
	unit, _ := v.unitOf(id)
	for _, f := range v.unitFindingsIn(unit, unreadyCallers) {
		for _, text := range v.callTexts(f) {
			k, ok := dependencyKindIn(text)
			if !ok {
				continue
			}
			byEndpoint := v.kindEndpoints[k.Kind]
			if byEndpoint == nil {
				byEndpoint = map[string]timeValue{}
				v.kindEndpoints[k.Kind] = byEndpoint
			}
			began := byEndpoint[call.endpoint]
			if !f.Since.IsZero() {
				began = began.earlier(f.Since)
			}
			byEndpoint[call.endpoint] = began
			return
		}
	}
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

// calledEndpoint is the endpoint the failing pod names, else the one
// its kind-only failure is the same failure of (kindOnlyEndpoint).
func (v *view) calledEndpoint(pod inventory.EntityID) (string, bool) {
	if call, ok := v.callOf(pod); ok {
		return call.endpoint, true
	}
	return v.kindOnlyEndpoint(pod)
}

// calledClass is how the effect's call to endpoint failed: the class
// its words name, or, for a kind-only failure attached to the
// endpoint, how the endpoint's own callers fail.
func (v *view) calledClass(
	effect inventory.EntityID, endpoint string,
) (detection.Mode, bool) {
	if call, ok := v.callOf(effect); ok {
		return call.class, call.endpoint == endpoint
	}
	attached, ok := v.kindOnlyEndpoint(effect)
	if !ok || attached != endpoint {
		return "", false
	}
	return endpointUnreachable, true
}

// ConnectionFailure reports text that says a connection or a name
// lookup failed, in any client's words (connectionFailure), including a
// lookup the DNS server could not answer.
func ConnectionFailure(text string) bool {
	lower := strings.ToLower(capText(text, dependencyScanMax))
	return connectionFailure.MatchString(lower) ||
		dnsServerFailure.MatchString(lower)
}
