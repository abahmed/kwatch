package investigate

import (
	"context"
	"sort"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// maxInvestigatedContainers bounds log reads per crash investigation.
// Replicas usually fail the same way; three are enough to see that.
const maxInvestigatedContainers = 3

// readCrash reads the previous run of the incident's crashing
// containers: the termination message from the model, the first
// meaningful error line and a short excerpt from the logs. Replicas that
// fail with the same signature are shown once.
func readCrash(
	ctx context.Context, s Sources, p incident.Incident,
) Result {
	var r Result
	seen := map[signatureKey]bool{}
	add := func(fact, text string) {
		key := signatureKey{fact: fact, signature: logSignature(text)}
		if text != "" && !kube.IsRuntimeLogFailure(text) && !seen[key] {
			seen[key] = true
			r.Evidence = append(r.Evidence,
				incident.Fact{Kind: fact, Text: text})
		}
	}
	for _, c := range crashContainers(s.Model, p) {
		if ctx.Err() != nil {
			break
		}
		// The termination message is in the model: no API call.
		add(incident.FactTermination,
			attributeText(s.Model, c, kube.AttrLastMessage))
		if s.Logs == nil {
			continue
		}
		lines := s.Logs(ctx, c)
		add(incident.FactError, kube.FirstErrorLine(lines))
		r.Output = appendUnique(r.Output, kube.ErrorExcerpt(lines), seen)
	}
	return r
}

// signatureKey identifies one fact or output line by its log
// signature, so replicas that fail the same way are shown once.
type signatureKey struct {
	fact, signature string
}

// outputFact is the signatureKey fact of quoted output lines.
const outputFact = "output"

// appendUnique appends the lines whose signature is not yet in seen.
func appendUnique(
	out, lines []string, seen map[signatureKey]bool,
) []string {
	for _, line := range lines {
		key := signatureKey{fact: outputFact, signature: logSignature(line)}
		if !seen[key] {
			seen[key] = true
			out = append(out, line)
		}
	}
	return out
}

// logSignature normalizes a log line so the same error from different
// replicas, runs or requests compares equal. The rules are shared with
// the root-cause engine.
func logSignature(line string) string {
	return format.Signature(line)
}

// crashContainers are the containers a crash investigation reads: the
// most severe container members, then the containers of the incident
// that ended with an error exit, at most maxInvestigatedContainers.
func crashContainers(
	model inventory.Reader, p incident.Incident,
) []inventory.EntityID {
	var out []inventory.EntityID
	seen := map[inventory.EntityID]bool{}
	for _, c := range topContainers(p) {
		seen[c.Entity] = true
		out = append(out, c.Entity)
	}
	for _, id := range erroredContainers(model, p) {
		if !seen[id] && len(out) < maxInvestigatedContainers {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// topContainers returns the incident's most severe container findings,
// at most maxInvestigatedContainers of them, each container once.
func topContainers(p incident.Incident) []detection.Finding {
	var containers []detection.Finding
	for key, s := range p.Members {
		if key.Entity.Kind == kube.KindContainer {
			containers = append(containers, s)
		}
	}
	sort.Slice(containers, func(i, j int) bool {
		if containers[i].Severity != containers[j].Severity {
			return containers[i].Severity > containers[j].Severity
		}
		return containers[i].Entity.String() <
			containers[j].Entity.String()
	})
	var out []detection.Finding
	seen := map[inventory.EntityID]bool{}
	for _, c := range containers {
		if !seen[c.Entity] && len(out) < maxInvestigatedContainers {
			seen[c.Entity] = true
			out = append(out, c)
		}
	}
	return out
}

// erroredContainers are the incident's containers, the members and those
// of its pods, that restarted after a run that ended with a non-zero
// exit code, as the model records it; in a stable order.
func erroredContainers(
	model inventory.Reader, p incident.Incident,
) []inventory.EntityID {
	var candidates []inventory.EntityID
	for key := range p.Members {
		if key.Entity.Kind == kube.KindContainer {
			candidates = append(candidates, key.Entity)
		}
	}
	for _, pod := range incidentPods(model, p) {
		candidates = append(candidates, containersOf(model, pod)...)
	}
	sortIDs(candidates)
	var out []inventory.EntityID
	seen := map[inventory.EntityID]bool{}
	for _, id := range candidates {
		if !seen[id] && errorExit(model, id) {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// errorExit reports a container that restarted after a non-zero exit.
func errorExit(model inventory.Reader, id inventory.EntityID) bool {
	e, ok := model.Entity(id)
	if !ok {
		return false
	}
	code, hasCode := e.Attribute(kube.AttrLastExitCode)
	exit, _ := code.Value.AsNumber()
	restarts, _ := e.Attribute(kube.AttrRestarts)
	count, _ := restarts.Value.AsNumber()
	return hasCode && exit != 0 && count >= 1
}
