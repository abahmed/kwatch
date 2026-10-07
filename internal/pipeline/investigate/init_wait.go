package investigate

import (
	"context"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// initWaitModes are the failure modes of an init container that runs
// too long.
var initWaitModes = []string{"InitWaiting"}

// maxInitLines is how many of the init container's last lines are quoted.
const maxInitLines = 2

func isInitWaitRoot(p incident.Incident) bool {
	return hasMode(p, initWaitModes)
}

// readInitWait quotes the last lines the stuck init container wrote, from
// its current run. When the container's configuration did not name the
// Service it waits for, the Service its output names is a fact of its
// own. The lines are quoted as written, never interpreted.
func readInitWait(
	ctx context.Context, s Sources, p incident.Incident,
) Result {
	var r Result
	for _, c := range initWaiting(p) {
		if ctx.Err() != nil || s.CurrentLogs == nil {
			break
		}
		lines := s.CurrentLogs(ctx, c.Entity)
		if len(lines) > maxInitLines {
			lines = lines[len(lines)-maxInitLines:]
		}
		r.Output = appendUnique(r.Output, lines,
			map[signatureKey]bool{})
		if !namesService(s.Model, c.Entity) {
			r.Evidence = append(r.Evidence, serviceInOutput(
				s.Model, c.Entity, lines)...)
		}
		break
	}
	return r
}

// initWaiting returns the incident's stuck init containers, the first
// by name first.
func initWaiting(p incident.Incident) []detection.Finding {
	var out []detection.Finding
	for _, c := range topContainers(p) {
		if modeIn(string(c.Mode), initWaitModes) {
			out = append(out, c)
		}
	}
	return out
}

// namesService reports whether the container's own configuration names
// a Service, so its finding already said what it waits for.
func namesService(model inventory.Reader, id inventory.EntityID) bool {
	e, ok := model.Entity(id)
	return ok && len(kube.ServiceCalls(e)) > 0
}

// serviceInOutput names the first existing Service the lines mention.
func serviceInOutput(
	model inventory.Reader, container inventory.EntityID, lines []string,
) []incident.Fact {
	for _, line := range lines {
		for _, ref := range kube.ServiceRefsIn(line, container.Namespace) {
			if !model.Exists(ref.Service) {
				continue
			}
			text := "Service " + ref.Service.Name + " in " +
				ref.Service.Namespace
			if noReadyEndpoints(model, ref.Service) {
				text += ", which has no ready endpoints"
			}
			return []incident.Fact{{Kind: incident.FactDependency,
				Subject: ref.Service.String(), Text: text}}
		}
	}
	return nil
}

// noReadyEndpoints reports a Service whose EndpointSlices list no ready
// endpoint. A Service with no slices known says nothing.
func noReadyEndpoints(
	model inventory.Reader, service inventory.EntityID,
) bool {
	slices := model.Related(service, inventory.Backs, inventory.Incoming)
	for _, id := range slices {
		slice, ok := model.Entity(id)
		if !ok {
			continue
		}
		attribute, _ := slice.Attribute(kube.AttrEndpointsReady)
		if ready, _ := attribute.Value.AsNumber(); ready > 0 {
			return false
		}
	}
	return len(slices) > 0
}
