package dashboard

import (
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// moreSymptoms word the failures familySymptoms leaves out, by the
// full mode first ("Missing.Secret") and then by family, so the "What's
// wrong?" line never falls back to the engine's title for them.
var moreSymptoms = map[string]symptom{
	"Missing.Secret": {"can't find a Secret it needs",
		"couldn't find a Secret it needs"},
	"Missing.ConfigMap": {"can't find a ConfigMap it needs",
		"couldn't find a ConfigMap it needs"},
	"Missing.ServiceAccount": {"can't find its ServiceAccount",
		"couldn't find its ServiceAccount"},
	"Missing": {"is missing something it needs",
		"was missing something it needs"},
	"Cert": {"has a certificate that has expired",
		"had a certificate that expired"},
	"Scaling": {"can't scale automatically",
		"couldn't scale automatically"},
	"Suspended": {"is suspended and not running",
		"was suspended"},
	"InvalidSchedule": {"has a schedule that can't be read",
		"had a schedule that couldn't be read"},
	"NoEndpoints": {"has no ready pods to send traffic to",
		"had no ready pods to send traffic to"},
	"BackendsDegraded": {"has some of its pods not ready for traffic",
		"had some pods not ready for traffic"},
	"BackendMissing": {"sends traffic to a Service that does not exist",
		"sent traffic to a Service that did not exist"},
	"PortMismatch": {"sends traffic to a port nothing listens on",
		"sent traffic to a port nothing listened on"},
	"LoadBalancerPending": {"is still waiting for its load balancer",
		"was waiting for its load balancer"},
	"Node": {"is not working properly", "was not working properly"},
	"NetworkUnavailable": {"has no working network",
		"had no working network"},
	"Unreachable": {"can't be reached", "couldn't be reached"},
	"Terminating": {"is stuck shutting down", "was stuck shutting down"},
}

// symptomFor is the symptom of p's failure: its mode, then its family.
func symptomFor(p incident.Incident) (symptom, bool) {
	mode, family := string(p.Mode), modeFamily(p)
	if s, ok := moreSymptoms[mode]; ok {
		return s, true
	}
	if s, ok := familySymptoms[family]; ok {
		return s, true
	}
	s, ok := moreSymptoms[family]
	return s, ok
}

// newVersionWhat is the sentence of a release whose new version fails
// while the previous one still serves, from the same facts as the
// title: "The new version of api crashes every time it starts. The
// previous version is still serving users." It says what the new
// version does, never that a rollout is stuck.
func newVersionWhat(d *IncidentDetail, p incident.Incident) string {
	past := p.State == incident.Resolved
	s := newVersionSymptom(d, p)
	text, serving := s.now, "is still serving users"
	if past {
		text, serving = s.was, "kept serving users"
	}
	return "The new version of " + p.Root.Name + " " + text +
		". The previous version " + serving + "."
}

// newVersionSymptom is what the new version does: the family's own
// failure when it has one, else a failing probe, a fatal line at start,
// and last that it keeps failing to start.
func newVersionSymptom(d *IncidentDetail, p incident.Incident) symptom {
	if s, ok := newVersionFamilies[modeFamily(p)]; ok {
		return s
	}
	if probe, ok := failingProbe(d); ok {
		return symptom{"keeps failing its " + probe + " check",
			"kept failing its " + probe + " check"}
	}
	if p.Cause != nil && explain.FatalStartLine(p.Cause.SharedError) {
		return symptom{"crashes every time it starts",
			"crashed every time it started"}
	}
	switch modeFamily(p) {
	case "CrashLoop", "Restarting", "Error", "Killed":
		return symptom{"keeps crashing", "kept crashing"}
	}
	return symptom{"keeps failing to start", "kept failing to start"}
}

// newVersionFamilies are the families whose own failure says more than
// a crash does.
var newVersionFamilies = map[string]symptom{
	"ImagePull": {"can't pull its image", "couldn't pull its image"},
	"OOMKilled": {"keeps running out of memory", "ran out of memory"},
	"CreateError": {"can't start its container",
		"couldn't start its container"},
	"InitError": {"can't finish starting", "couldn't finish starting"},
	"Hook":      {"fails its start hook", "failed its start hook"},
	"Unschedulable": {"can't be placed on a node",
		"couldn't be placed on a node"},
	"Pending":       {"can't be placed on a node", "wasn't placed on a node"},
	"NotScheduling": {"can't be placed on a node", "wasn't placed on a node"},
	"NotReady":      {"never becomes ready", "never became ready"},
}
