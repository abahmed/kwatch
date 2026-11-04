package investigate

import (
	"context"
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// probeLogCheck reads what a running container whose probe gets an
// HTTP error status says about itself in its current log. It comes
// after the crash check: a container that crashed explains itself in
// its previous run.
func probeLogCheck() check {
	leads := func(p incident.Incident, model inventory.Reader) bool {
		return len(probeLogContainers(p, model)) > 0
	}
	return check{name: incident.CheckProbeLog,
		title: "Probe failure output", budget: apiReadBudget,
		applies: leads, leads: leads, summary: readProbeLog}
}

// readProbeLog quotes the last line naming a failure in the current log
// of each such container: the line the crash-log round already read
// when the model has it (no API call), else a read of the current run.
// Replicas that say the same thing are quoted once.
func readProbeLog(
	ctx context.Context, s Sources, p incident.Incident,
) Result {
	var r Result
	seen := map[string]bool{}
	for _, c := range probeLogContainers(p, s.Model) {
		if ctx.Err() != nil {
			break
		}
		line := attributeText(s.Model, c, kube.AttrProbeLogLine)
		if line == "" && s.CurrentLogs != nil {
			line = kube.LastFailureLine(s.CurrentLogs(ctx, c))
		}
		key := logSignature(line)
		if line == "" || kube.IsRuntimeLogFailure(line) || seen[key] {
			continue
		}
		seen[key] = true
		r.Evidence = append(r.Evidence, incident.Fact{
			Kind: incident.FactProbeLog, Subject: c.String(), Text: line})
	}
	return r
}

// probeLogContainers are the incident's running member containers whose
// probe finding says the application answered with an HTTP error
// status, in a stable order, at most maxInvestigatedContainers.
func probeLogContainers(
	p incident.Incident, model inventory.Reader,
) []inventory.EntityID {
	var out []inventory.EntityID
	seen := map[inventory.EntityID]bool{}
	for key, f := range p.Members {
		id := key.Entity
		if id.Kind == kube.KindContainer && !seen[id] &&
			answeredError(f) && running(model, id) {
			seen[id] = true
			out = append(out, id)
		}
	}
	sortIDs(out)
	if len(out) > maxInvestigatedContainers {
		out = out[:maxInvestigatedContainers]
	}
	return out
}

// answeredError reports a probe finding whose result is an HTTP error
// status ("HTTP 500"): the application answered and failed its own
// check. A probe that got no answer is left to the crash check.
func answeredError(f detection.Finding) bool {
	for _, e := range f.Evidence {
		if e.Label != detection.EvidenceProbeResult {
			continue
		}
		code, ok := strings.CutPrefix(e.Value, "HTTP ")
		status, err := strconv.Atoi(code)
		return ok && err == nil && status >= 400
	}
	return false
}

// running reports a container the model holds as running.
func running(model inventory.Reader, id inventory.EntityID) bool {
	e, ok := model.Entity(id)
	return ok && textOf(e, kube.AttrState) == "running"
}
