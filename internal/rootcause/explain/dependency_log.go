package explain

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// A crashing workload whose own last words say it could not connect to
// a dependency ("RedisConnectionException: It was not possible to
// connect", "PSQLException: Connection to db:5432 refused", "dial tcp
// cache:6379: connect: connection refused") is explained by that
// dependency, as its log names it. Words alone are never proof: the row
// is capped below high confidence, and a dependency that is an in-cluster
// Service with no ready endpoints (or failing backends) is left to the
// called-service rows, which have the structure to show it.

// DependencyLogRow is the row that blames a dependency the log names.
const DependencyLogRow = "log-names-dependency"

// Pseudo modes of a workload whose log names a dependency it cannot
// reach. The finer mode says the host is a Service of the cluster.
const (
	ModeDependencyUnreachable detection.Mode = "Dependency.Unreachable"
	ModeDependencyService     detection.Mode = "Dependency.Unreachable." +
		"Service"
)

// DependencyLogMaxConfidence caps a cause read from log text alone: it
// is likely, never certain.
const DependencyLogMaxConfidence = ConfidenceHigh - 0.01

var dependencyLogRows = []Row{{
	Name: DependencyLogRow,
	Cause: Side{Group: AnyGroup, Kind: AnyKind,
		Modes: []detection.Mode{ModeDependencyUnreachable}},
	Link: LinkSelf,
	// A pod between crashes fails its probes or is not ready; the
	// crash is required of the text (dependencyOf), not of the mode.
	Effect: podSide(callerFailuresAnd(detection.ModeProbe,
		detection.ModeNotReady)...),
	Prior: 0.6, Inside: true,
}}

// Dependency is what a failed connection's text names: the kind of
// service ("PostgreSQL"), the host it called ("db:5432"), or both.
type Dependency struct {
	Name string
	Host string
}

// Label is the dependency in words: "PostgreSQL at db:5432", "Redis",
// or the host alone.
func (d Dependency) Label() string {
	switch {
	case d.Name != "" && d.Host != "":
		return d.Name + " at " + d.Host
	case d.Name != "":
		return d.Name
	}
	return d.Host
}

// DependencyIn reads the dependency a failed connection's text names.
// It reports false unless the text says a connection failed and names
// what it was to: a known client or a host.
func DependencyIn(text string) (Dependency, bool) {
	lower := withoutUserinfo(strings.ToLower(text))
	failures := connectionFailure.FindAllStringIndex(lower, -1)
	if failures == nil || dnsServerFailure.MatchString(lower) {
		return Dependency{}, false
	}
	host, port := dependencyHost(lower, failures)
	d := Dependency{Name: clientName(lower, port, failures), Host: host}
	if port != "" && d.Host != "" {
		d.Host += ":" + port
	}
	return d, d.Name != "" || d.Host != ""
}

// dependencyModes are the pseudo modes of effect's workload when the
// error text of its crashing unit names a dependency.
func (v *view) dependencyModes(effect inventory.EntityID) []detection.Mode {
	unit, ok := v.unitOf(effect)
	if !ok {
		return nil
	}
	d, ok := v.dependencyOf(unit)
	if !ok || v.dependencyShared(d) || v.dependencyProbed(unit) {
		// Several workloads failing on one dependency are a pattern
		// the shared rows (endpoint, signature, node) weigh, and a
		// dependency kwatch probes is judged by the probe.
		return nil
	}
	if service, ok := v.dependencyService(d, unit); ok {
		if v.noReadyEndpoints(service) || v.backendsFailing(service) {
			// The called-service rows show it with structure.
			return nil
		}
		return []detection.Mode{ModeDependencyService}
	}
	if d.Host != "" {
		// A host outside the cluster that one workload alone fails on
		// is that workload's own problem (its address, its
		// credentials) until others fail on it too or kwatch's probe
		// of it fails: the endpoint rows judge it.
		return nil
	}
	return []detection.Mode{ModeDependencyUnreachable}
}

// dependencyOf is the dependency the crashing unit's own error text
// names: the termination message or previous log line of a container
// that crashed or exited, never the kubelet's probe message.
func (v *view) dependencyOf(unit inventory.EntityID) (Dependency, bool) {
	for _, f := range v.unitFindings(unit) {
		for _, text := range v.callTexts(f) {
			if d, ok := DependencyIn(text); ok {
				return d, true
			}
		}
	}
	// Between crashes the container runs and fails its probes: its last
	// run still ended with an error exit, and its words still count.
	for _, id := range v.s.Model.Related(unit, inventory.PartOf,
		inventory.Incoming) {
		if !v.failing(id) || !restartedAfterError(v.s.Model, id) {
			continue
		}
		e, _ := v.s.Model.Entity(id)
		for _, attr := range []string{kube.AttrLastMessage,
			kube.AttrLastErrorLine} {
			if d, ok := DependencyIn(attrText(e, attr)); ok {
				return d, true
			}
		}
	}
	return Dependency{}, false
}

// restartedAfterError reports a container that restarted after a run
// that ended with a non-zero exit code.
func restartedAfterError(model inventory.Reader, id inventory.EntityID) bool {
	e, ok := model.Entity(id)
	if !ok {
		return false
	}
	code, _ := attrNumber(e, kube.AttrLastExitCode)
	restarts, _ := attrNumber(e, kube.AttrRestarts)
	return code != 0 && restarts >= 1
}

// dependencyLine is the text a dependency cause was read from, for the
// message to quote: the first own error line of the root's failing pods
// that names a dependency.
func (s Snapshot) dependencyLine(c Cause) string {
	if c.Row != DependencyLogRow {
		return ""
	}
	for _, id := range sortedFindingIDs(s) {
		if !ownedBy(s, id, c.Root) {
			continue
		}
		for _, text := range s.ownTexts(id) {
			if _, ok := DependencyIn(text); ok {
				return text
			}
		}
	}
	return ""
}

// ownTexts are what id's findings quote of the program, without the
// kubelet's probe message, then its recorded previous-log line.
func (s Snapshot) ownTexts(id inventory.EntityID) []string {
	var out []string
	for _, f := range s.Findings[id] {
		for _, e := range f.Evidence {
			if e.Label != probeEvidenceLabel {
				out = append(out, e.Value)
			}
		}
	}
	if line := lastErrorLine(s, id); line != "" {
		out = append(out, line)
	}
	return out
}

// dependencyService is the existing Service of the cluster the host
// names, if any. The Service gates the unit.
func (v *view) dependencyService(
	d Dependency, unit inventory.EntityID,
) (inventory.EntityID, bool) {
	host := endpointHost(d.Host)
	if host == "" {
		return inventory.EntityID{}, false
	}
	service, ok := clusterServiceID(host, unit)
	if !ok {
		return inventory.EntityID{}, false
	}
	v.gate(unit, service)
	return service, v.s.Model.Exists(service)
}

// lastErrorLine is the previous-log line the crash-log round recorded
// on a container.
func lastErrorLine(s Snapshot, id inventory.EntityID) string {
	if id.Kind != kube.KindContainer {
		return ""
	}
	e, ok := s.Model.Entity(id)
	if !ok {
		return ""
	}
	return attrText(e, kube.AttrLastErrorLine)
}

// dependencyLogOnly reports a candidate blamed only through the
// dependency row: what it stands on is text.
func dependencyLogOnly(c *candidate) bool {
	found := false
	for _, how := range c.covers {
		if how.derived || how.match.row.Name == selfRow.Name {
			continue
		}
		if how.match.row.Name != DependencyLogRow {
			return false
		}
		found = true
	}
	return found
}

// dependencyKey identifies a dependency across workloads: its host,
// else its kind.
func dependencyKey(d Dependency) string {
	if host := endpointHost(d.Host); host != "" {
		return host
	}
	return d.Name
}

// dependencyShared reports a dependency the failing pods of more than
// one workload name.
func (v *view) dependencyShared(d Dependency) bool {
	return len(v.dependencyDependents()[dependencyKey(d)]) > 1
}

// dependencyDependents maps each dependency the failing pods name to
// the workloads (top owners) of those pods.
func (v *view) dependencyDependents() map[string]map[inventory.EntityID]bool {
	if v.depUsers != nil {
		return v.depUsers
	}
	v.depUsers = map[string]map[inventory.EntityID]bool{}
	seen := map[inventory.EntityID]bool{}
	for _, id := range sortedFindingIDs(v.s) {
		unit, ok := v.unitOf(id)
		if !ok || seen[unit] {
			continue
		}
		seen[unit] = true
		d, ok := v.dependencyOf(unit)
		if !ok {
			continue
		}
		key := dependencyKey(d)
		if v.depUsers[key] == nil {
			v.depUsers[key] = map[inventory.EntityID]bool{}
		}
		v.depUsers[key][rootcause.TopOwner(v.s.Model, unit)] = true
	}
	return v.depUsers
}

// dependencyProbed reports a unit whose failed call names an external
// endpoint kwatch probes: the probe's answer, not the text, decides.
func (v *view) dependencyProbed(unit inventory.EntityID) bool {
	call, ok := v.callOf(unit)
	if !ok {
		return false
	}
	id := inventory.CoreID(KindExternalEndpoint, "", call.endpoint)
	e, ok := v.s.Model.Entity(id)
	if !ok {
		return v.failsOnOneNodeOnly(unit)
	}
	_, known := e.Attribute(kube.AttrHealthy)
	return known || v.failsOnOneNodeOnly(unit)
}
