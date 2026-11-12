package dashboard

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/redact"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// symptom is what is wrong in present and past tense, after the
// object's name: "keeps crashing", "was crashing".
type symptom struct{ now, was string }

// familySymptoms word the failure families a reader sees first. A
// family not listed here falls back to the headline.
var familySymptoms = map[string]symptom{
	"CrashLoop":   {"keeps crashing", "was crashing"},
	"Restarting":  {"keeps restarting", "kept restarting"},
	"Error":       {"exits with an error", "exited with an error"},
	"Killed":      {"was killed", "was killed"},
	"OOMKilled":   {"keeps running out of memory", "ran out of memory"},
	"ImagePull":   {"can't pull its image", "couldn't pull its image"},
	"CreateError": {"can't start its container", "couldn't start"},
	"InitError":   {"can't finish starting", "couldn't finish starting"},
	"InitWaiting": {"is stuck starting", "was stuck starting"},
	"Hook":        {"fails its start hook", "failed its start hook"},
	"NotReady":    {"is not ready", "was not ready"},
	"Unschedulable": {"is stuck pending: no node can run it",
		"was stuck pending"},
	"Pending":          {"is stuck pending", "was stuck pending"},
	"NotScheduling":    {"is stuck pending", "was stuck pending"},
	"SchedulingGated":  {"is held before scheduling", "was held"},
	"Evicted":          {"had pods evicted", "had pods evicted"},
	"JobFailed":        {"failed", "failed"},
	"DeadlineExceeded": {"ran too long and was stopped", "ran too long"},
	"RolloutStuck": {"has a new version that is not getting ready",
		"had a new version that never got ready"},
	"Unavailable": {"has too few pods available",
		"had too few pods available"},
	"ReplicaFailure": {"can't create its pods", "couldn't create pods"},
	"MemoryPressure": {"is low on memory", "was low on memory"},
	"DiskPressure":   {"is low on disk", "was low on disk"},
	"PIDPressure":    {"is low on process IDs", "was low on process IDs"},
	"Draining":       {"is being drained", "was drained"},
	"StuckDeleting":  {"is stuck deleting", "was stuck deleting"},
}

// probeSymptoms word a failing probe.
var probeSymptoms = map[string]symptom{
	"readiness": {"is failing its readiness check, so it gets no " +
		"traffic", "was failing its readiness check"},
	"liveness": {"is failing its liveness check and being restarted",
		"was failing its liveness check"},
	"startup": {"is failing its startup check",
		"was failing its startup check"},
}

// whatOf is one plain sentence of what is wrong, the visible symptom
// first: no endpoint serving, a failing probe, the failure family,
// else the headline. When that says nothing the title does not, the
// latest finding's own sentence takes its place ("2 of 3 Service
// backends are ready").
func whatOf(d *IncidentDetail, p incident.Incident) string {
	if lead, ok := leftoverWhat(p); ok {
		// No member's sentence is the root's: each says its own in
		// the leftovers list.
		return lead
	}
	what := symptomOf(d, p)
	if bare(what) != "" && bare(what) != bare(d.Title) {
		return what
	}
	if f := findingText(p, d.Title); f != "" {
		return sentence(f)
	}
	return what
}

// findingText is the summary of the latest finding the incident
// explains, the root's first, that is not the title again; "" without
// one. A resolved incident keeps its last findings.
func findingText(p incident.Incident, title string) string {
	var best detection.Finding
	found := false
	for _, f := range p.Members {
		text := redact.Evidence(f.Summary)
		if f.Advisory || bare(text) == "" || bare(text) == bare(title) {
			continue
		}
		f.Summary = text
		if !found || laterFinding(f, best, p.Root) {
			best, found = f, true
		}
	}
	return best.Summary
}

// laterFinding orders findings: the root's first, then the latest, then
// by summary so the choice is stable.
func laterFinding(a, b detection.Finding, root inventory.EntityID) bool {
	if ar, br := a.Entity == root, b.Entity == root; ar != br {
		return ar
	}
	if !a.Since.Equal(b.Since) {
		return a.Since.After(b.Since)
	}
	return a.Summary < b.Summary
}

// bare is s in lower case with only its letters and digits, words
// apart: two texts that differ in case or punctuation compare equal.
func bare(s string) string {
	return strings.Join(strings.FieldsFunc(strings.ToLower(s),
		func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r)
		}), " ")
}

// symptomOf is the symptom sentence of whatOf, before the finding
// fallback.
func symptomOf(d *IncidentDetail, p incident.Incident) string {
	past := p.State == incident.Resolved
	subject := subjectOf(p)
	if s, ok := notServing(d, past); ok && subject != "" {
		return sentence(subject + " " + s)
	}
	if p.PreviousServes && p.Root.Name != "" {
		return newVersionWhat(d, p)
	}
	s, ok := symptomFor(p)
	if probe, found := failingProbe(d); found &&
		(probe != "liveness" || !crashedOnItsOwn(d)) {
		// A liveness probe that fails after the container crashed by
		// itself is a symptom: the crash leads.
		s, ok = probeSymptoms[probe], true
	}
	if !ok || subject == "" {
		return sentence(d.Title)
	}
	text := s.now
	if past {
		text = s.was
	}
	return sentence(subject + " " + text + extentOf(d, p, past))
}

// subjectOf names the root as a reader says it: "Deployment api in
// shop", "Node n1".
// A virtual scheduling root names the scheduler's reason, not an
// object: the title says it in words instead.
func subjectOf(p incident.Incident) string {
	if p.Root.Name == "" || p.Root.Kind == rootcause.KindScheduling {
		return ""
	}
	return rootWords(p.Root)
}

// notServing words a Service with endpoints and none ready.
func notServing(d *IncidentDetail, past bool) (string, bool) {
	for _, r := range d.Traffic {
		if r.Kind != "Service" || r.Total == 0 || r.Ready > 0 {
			continue
		}
		verb := "is"
		if past {
			verb = "was"
		}
		return fmt.Sprintf("%s not serving traffic: Service %s has 0 of "+
			"%d endpoints ready", verb, r.Name, r.Total), true
	}
	return "", false
}

// failingProbe is the probe that fails, readiness first since it is
// what takes a pod out of traffic.
func failingProbe(d *IncidentDetail) (string, bool) {
	found := ""
	for _, r := range d.Probes {
		if r.Probe == "readiness" {
			return r.Probe, true
		}
		if found == "" {
			found = r.Probe
		}
	}
	_, ok := probeSymptoms[found]
	return found, ok
}

// extentOf adds how far it goes, when known and still going: restarts
// of a crashing container, pods waiting for a node.
func extentOf(d *IncidentDetail, p incident.Incident, past bool) string {
	if past {
		return ""
	}
	switch modeFamily(p) {
	case "CrashLoop", "Restarting", "OOMKilled", "Error":
		if n := mostRestarts(d); n > 1 {
			return fmt.Sprintf(" (%d restarts)", n)
		}
	case "Unschedulable", "Pending", "NotScheduling":
		if d.Waiting != nil && d.Waiting.Pods > 1 {
			return fmt.Sprintf(" (%d pods waiting)", d.Waiting.Pods)
		}
	}
	return ""
}

// mostRestarts is the highest restart count of the containers now, or
// of the latest break when kwatch has no live view.
func mostRestarts(d *IncidentDetail) int {
	n := 0
	for _, c := range containerRowsOf(d) {
		n = max(n, c.Restarts)
	}
	return n
}

// containerRowsOf is the containers now, else those of the latest
// break.
func containerRowsOf(d *IncidentDetail) []ContainerRow {
	if len(d.Containers) > 0 || d.Snapshot == nil {
		return d.Containers
	}
	return d.Snapshot.Containers
}

// sentence ends s with a full stop unless it already ends a sentence.
func sentence(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.HasSuffix(s, ".") || strings.HasSuffix(s, "!") ||
		strings.HasSuffix(s, "?") || strings.HasSuffix(s, "…") {
		return s
	}
	return s + "."
}
