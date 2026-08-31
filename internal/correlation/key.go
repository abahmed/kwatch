package correlation

import (
	"strings"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

// Incident keys follow the format documented on model.IncidentKey. This file
// is the single source of truth for building and parsing them, and for the
// "ns/name" owner encoding used by workload-object detectors.

// BuildKey constructs the incident key used for dedup, grouping, and baseline.
func BuildKey(namespace, owner, reason, container string) model.IncidentKey {
	return model.IncidentKey(namespace + ":" + owner + ":" + reason + ":" + container)
}

// incidentOwner returns a stable logical owner only for generated Pods that
// have no Kubernetes owner reference. A plain ownerless Pod remains keyed by
// its actual name because Kubernetes provides no evidence that another Pod is
// its replacement.
func incidentOwner(ev event.Event, owner string) string {
	if ev.Resource == "pod" && ev.OwnerKind == "" &&
		ev.PodGenerateName != "" && owner == ev.PodName {
		return "generated/" + ev.PodGenerateName
	}
	return owner
}

// GlobalKey builds the cluster-scoped key form "<reason>|global|<scope>" used
// when an incident is shared across namespaces (e.g. ImagePullBackOff rate
// limits, timeouts, DNS, TLS). The key intentionally omits namespace and owner
// so the same underlying problem maps to a single incident wherever it fires.
func GlobalKey(reason, scope string) model.IncidentKey {
	return model.IncidentKey(reason + globalSeparator + scope)
}

// KeyParts is the parsed form of an incident key. Fields that are absent in
// the key's form are left empty (e.g. Owner for a global key).
type KeyParts struct {
	Namespace string
	Owner     string
	Reason    string
	Container string

	IsGlobal bool // key uses the "<reason>|global|<scope>" form
	Scope    string

	IsGroup  bool // key is a smart-group incident ("__group__:<group key>")
	GroupKey string

	IsMassFailure     bool // key is a mass-failure incident (mass-failure/<dependency>)
	MassDependencyKey string
}

// ParseKey decomposes an incident key. Global and group keys do not follow
// the "ns:owner:reason:container" layout and are reported via IsGlobal and
// IsGroup instead of being split into parts.
func ParseKey(key model.IncidentKey) KeyParts {
	s := string(key)
	if strings.HasPrefix(s, groupKeyPrefix) {
		return KeyParts{IsGroup: true, GroupKey: strings.TrimPrefix(s, groupKeyPrefix)}
	}
	if strings.HasPrefix(s, massFailureKeyPrefix) {
		return KeyParts{IsMassFailure: true, MassDependencyKey: strings.TrimPrefix(s, massFailureKeyPrefix)}
	}
	if i := strings.Index(s, globalSeparator); i >= 0 {
		return KeyParts{IsGlobal: true, Reason: s[:i], Scope: s[i+len(globalSeparator):]}
	}
	parts := strings.Split(s, ":")
	p := KeyParts{}
	switch len(parts) {
	case 4:
		p.Container = parts[3]
		fallthrough
	case 3:
		p.Reason = parts[2]
		fallthrough
	case 2:
		p.Owner = parts[1]
		fallthrough
	case 1:
		p.Namespace = parts[0]
	}
	return p
}

// IsGlobalKey reports whether the key uses the cross-namespace global form.
func IsGlobalKey(key model.IncidentKey) bool {
	return strings.Contains(string(key), globalSeparator)
}

// IsGroupKey reports whether the key identifies a smart-group incident.
func IsGroupKey(key model.IncidentKey) bool {
	return strings.HasPrefix(string(key), groupKeyPrefix)
}

// IsMassFailureKey reports whether the key identifies a mass-failure incident.
func IsMassFailureKey(key model.IncidentKey) bool {
	return strings.HasPrefix(string(key), massFailureKeyPrefix)
}

// MassFailureKey builds the incident key for a mass-failure on the given
// shared dependency ("kind/namespace/name"). The stable key lets the engine
// dedup, persist, and resume mass-failure alerts across restarts.
func MassFailureKey(dependencyKey string) model.IncidentKey {
	return model.IncidentKey(massFailureKeyPrefix + dependencyKey)
}

// groupKeyPrefix marks incident keys synthesized by smart grouping.
const groupKeyPrefix = "__group__:"

// massFailureKeyPrefix marks incident keys synthesized by the mass-failure
// detector. They are persisted alongside real incidents but kept out of the
// normal lifecycle (no renotify, no baseline participation).
const massFailureKeyPrefix = "mass-failure/"

// globalSeparator delimits the cluster-scoped key form "<reason>|global|<scope>".
const globalSeparator = "|global|"

// OwnerPath returns the fully-qualified "namespace/name" form of an owner,
// used by workload-object detectors for Incident.Name and the owner slot of
// BuildKey. Pod symptoms use the bare owner name instead — see the
// encoding table on model.Incident.Name.
func OwnerPath(namespace, name string) string {
	if namespace == "" {
		return name
	}
	return namespace + "/" + name
}

// SplitOwnerPath returns the namespace and bare name of an "ns/name" owner
// path. Paths without a separator are treated as bare names in namespace "".
func SplitOwnerPath(path string) (namespace, name string) {
	if i := strings.LastIndex(path, "/"); i > 0 {
		return path[:i], path[i+1:]
	}
	return "", path
}
