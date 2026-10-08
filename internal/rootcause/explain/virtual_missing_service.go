package explain

import (
	"regexp"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// A failing pod that calls a Service of its own cluster that does not
// exist: a misspelt name in its configuration or in the errors it
// quotes ("lookup paymnts.shop.svc.cluster.local: no such host"). The
// missing Service is the cause. Healthy pods are never checked: a
// workload may not use the name yet.

// missingRef is a Service a failing pod names that does not exist.
type missingRef struct {
	service inventory.EntityID
	// port is the port the pod's configuration names, 0 when none.
	port int
	// line is the quoted error that shows the name failing, "" when
	// only the configuration names it.
	line string
}

// maxMissingRefs bounds the missing Services checked for one pod.
const maxMissingRefs = 3

// lookupFailures find the host a failed name lookup names, in
// lower-case text: Go's resolver, Node's getaddrinfo and Java's
// UnknownHostException. Group 1 is the host.
var lookupFailures = []*regexp.Regexp{
	regexp.MustCompile(`lookup ([a-z0-9][a-z0-9.-]*[a-z0-9])` +
		`(?: on \S+)?: no such host`),
	regexp.MustCompile(`getaddrinfo (?:enotfound|eai_again) ` +
		`([a-z0-9][a-z0-9.-]*[a-z0-9])`),
	regexp.MustCompile(`unknownhostexception: ([a-z0-9][a-z0-9.-]*[a-z0-9])`),
}

// missingServiceRows say that a Service the failing pods name, and
// that does not exist, explains them. The row for a quoted lookup is
// stronger than the one for a name only the configuration holds.
var missingServiceRows = []Row{
	{
		Name: "missing-service-called",
		Cause: Side{Kind: kube.KindService,
			Modes: []detection.Mode{ModeMissing}},
		Link: LinkCalls,
		Effect: Side{Kind: kube.KindPod, Modes: missingCallFailures,
			Signal: SignalDNS},
		Prior: 0.8,
	},
	{
		Name: "missing-service-configured",
		Cause: Side{Kind: kube.KindService,
			Modes: []detection.Mode{ModeMissing}},
		Link:   LinkCalls,
		Effect: Side{Kind: kube.KindPod, Modes: missingCallFailures},
		Prior:  0.6,
	},
}

// missingCallFailures are the failures a missing Service can cause: the
// crashes a failed call causes, and a pod that never becomes ready.
var missingCallFailures = append(
	append([]detection.Mode(nil), callerFailures...),
	detection.ModeNotReady, detection.ModeProbeReadiness)

// missingServiceHops lead from a failing pod to each Service it names
// that does not exist.
func (v *view) missingServiceHops(pod inventory.EntityID) []hop {
	if !v.unitGate(pod) || !v.s.synced(kube.KindService) {
		return nil
	}
	var out []hop
	for _, ref := range v.missingRefs(pod) {
		v.gate(pod, ref.service)
		out = append(out, hop{link: LinkCalls, to: ref.service})
	}
	return out
}

// missingCalledModes is ModeMissing for a Service the effect names and
// that does not exist.
func (v *view) missingCalledModes(
	id, effect inventory.EntityID, link LinkType,
) []modeHealth {
	if link != LinkCalls || v.s.Model.Exists(id) {
		return nil
	}
	unit, ok := v.unitOf(effect)
	if !ok || !v.s.synced(kube.KindService) {
		return nil
	}
	for _, ref := range v.missingRefs(unit) {
		if ref.service == id {
			return []modeHealth{{mode: ModeMissing,
				health: detection.Failing, pseudo: true}}
		}
	}
	return nil
}

// missingRefs are the Services that do not exist and that the pod's
// errors or configuration name, in a stable order. A Service an error
// names comes with its line, and with the port the configuration gives.
func (v *view) missingRefs(pod inventory.EntityID) []missingRef {
	var refs refList
	for _, ref := range v.quotedMissing(pod) {
		refs.add(ref)
	}
	if entity, ok := v.s.Model.Entity(pod); ok {
		quoted := v.crashErrors(pod)
		for _, call := range kube.ServiceCalls(entity) {
			if v.namesMissingService(call.Service, pod) &&
				namedOrSilent(quoted, call.Service) {
				refs.add(missingRef{service: call.Service, port: call.Port})
			}
		}
	}
	if len(refs.list) > maxMissingRefs {
		return refs.list[:maxMissingRefs]
	}
	return refs.list
}

// crashErrors are the errors the pod's containers left as their last
// words, as written: the termination message, or the first error line
// of the previous log. They are kept while the container runs again.
func (v *view) crashErrors(pod inventory.EntityID) []string {
	var out []string
	for _, id := range v.s.Model.Related(pod, inventory.PartOf,
		inventory.Incoming) {
		e, ok := v.s.Model.Entity(id)
		if !ok {
			continue
		}
		for _, name := range []string{kube.AttrLastMessage,
			kube.AttrLastErrorLine} {
			if a, ok := e.Attribute(name); ok &&
				a.Value.AsText() != "" {
				out = append(out, a.Value.AsText())
				break
			}
		}
	}
	return out
}

// namedOrSilent reports whether a name only the configuration holds may
// explain the crash: the quoted errors name the Service, or the pod
// quotes none. A crash that quotes a different error is its own cause;
// a name in its configuration does not outrank it.
func namedOrSilent(quoted []string, service inventory.EntityID) bool {
	if len(quoted) == 0 {
		return true
	}
	for _, line := range quoted {
		if strings.Contains(strings.ToLower(line), service.Name) {
			return true
		}
	}
	return false
}

// refList collects missing Services, one entry for each, merging what
// several sources say about it.
type refList struct {
	list []missingRef
	at   map[inventory.EntityID]int
}

func (r *refList) add(ref missingRef) {
	i, seen := r.at[ref.service]
	if !seen {
		if r.at == nil {
			r.at = map[inventory.EntityID]int{}
		}
		r.at[ref.service] = len(r.list)
		r.list = append(r.list, ref)
		return
	}
	known := &r.list[i]
	known.port = max(known.port, ref.port)
	if known.line == "" {
		known.line = ref.line
	}
}

// namesMissingService reports a Service that does not exist in a
// namespace that does, or in the pod's own.
func (v *view) namesMissingService(
	service, pod inventory.EntityID,
) bool {
	if v.s.Model.Exists(service) {
		return false
	}
	return service.Namespace == pod.Namespace || v.s.Model.Exists(
		inventory.CoreID(kube.KindNamespace, "", service.Namespace))
}

// quotedMissing are the missing Services the failing pod's own errors
// show a failed lookup of, each with the line, quoted as written.
func (v *view) quotedMissing(pod inventory.EntityID) []missingRef {
	var out []missingRef
	for _, line := range v.errorLines(pod) {
		lower := strings.ToLower(line)
		if dnsServerFailure.MatchString(lower) {
			continue
		}
		host := lookedUpHost(lower)
		ns, name, ok := kube.ClusterServiceName(host, pod.Namespace)
		if host == "" || !ok {
			continue
		}
		service := inventory.CoreID(kube.KindService, ns, name)
		if v.namesMissingService(service, pod) && !v.dnsUnwell(pod) {
			out = append(out, missingRef{service: service, line: line})
		}
	}
	return out
}

// dnsUnwell reports a cluster DNS that fails itself, by its own finding
// or by its servers': lookups fail then whether the name exists or not,
// and the DNS rows are the better cause. pod depends on the answer.
func (v *view) dnsUnwell(pod inventory.EntityID) bool {
	v.gate(pod, kube.ClusterDNS)
	if v.failing(kube.ClusterDNS) {
		return true
	}
	for id := range v.s.Findings {
		if v.dnsServer(id) && v.unitFailing(id) {
			return true
		}
	}
	return false
}

// lookedUpHost is the host a failed name lookup names, or "".
func lookedUpHost(lower string) string {
	for _, pattern := range lookupFailures {
		if m := pattern.FindStringSubmatch(lower); m != nil {
			return m[1]
		}
	}
	return ""
}

// errorLines are the error texts of a failing pod and its containers:
// the evidence of their findings and the recent events, one per line.
func (v *view) errorLines(pod inventory.EntityID) []string {
	ids := append([]inventory.EntityID{pod}, v.s.Model.Related(pod,
		inventory.PartOf, inventory.Incoming)...)
	sortIDs(ids)
	var out []string
	for _, id := range ids {
		for _, f := range v.s.Findings[id] {
			for _, e := range f.Evidence {
				out = append(out, e.Value)
			}
		}
		for _, note := range v.s.Model.Notes(id, v.since) {
			out = append(out, note.Message)
		}
	}
	return out
}

// scoreMissingCall says what the pods call: the port their
// configuration names, and a Service of the namespace whose name is
// very close to the missing one.
func scoreMissingCall(v *view, c *candidate) outcome {
	if c.id.Kind != kube.KindService || len(c.others()) == 0 ||
		c.bestMatch().link != LinkCalls || v.s.Model.Exists(c.id) {
		return outcome{}
	}
	port := 0
	for _, effect := range c.others() {
		unit, ok := v.unitOf(effect)
		if !ok {
			continue
		}
		// A workload that changed inside the window may have been given
		// the name by that change: the change is the better cause, and
		// it says what it set.
		if len(v.changesOf(rootcause.TopOwner(v.s.Model, unit))) > 0 {
			return outcome{veto: "the workload changed shortly " +
				"before, and the change explains the name"}
		}
		for _, ref := range v.missingRefs(unit) {
			if ref.service == c.id {
				port = max(port, ref.port)
			}
		}
	}
	var fields []string
	names := v.serviceNames(c.id.Namespace)
	if near := closestName(c.id.Name, names); near != "" {
		fields = []string{near}
	}
	return outcome{weight: MissingCallWeight,
		code: rootcause.ProofMissingCall, count: port, fields: fields,
		text: "the pods call a Service that does not exist"}
}

// MissingCallWeight is the support a missing called Service gets from
// the pods naming it.
const MissingCallWeight = 0.1

func init() {
	scorers = append(scorers, scorer{"missing-call",
		scoreMissingCall})
}

// serviceNames are the names of the Services of a namespace.
func (v *view) serviceNames(namespace string) []string {
	var out []string
	for _, id := range v.s.Model.EntitiesIn(kube.KindService, namespace) {
		out = append(out, id.Name)
	}
	return out
}
