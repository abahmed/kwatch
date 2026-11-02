package dashboard

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/redact"
)

// maxProbes bounds the failing probes listed.
const maxProbes = 4

// unhealthyReason is the kubelet's event reason for a failed probe.
const unhealthyReason = "Unhealthy"

// probeOfReason maps a probe finding to the probe it is about.
var probeOfReason = map[string]string{
	reasons.LivenessProbeFailed:  "liveness",
	reasons.LivenessKilled:       "liveness",
	reasons.ReadinessProbeFailed: "readiness",
	reasons.StartupProbeFailed:   "startup",
}

// probeKey is one probe of one container.
type probeKey struct {
	container inventory.EntityID
	probe     string
}

// probesOf lists the probes p's findings say keep failing, with their
// settings from the container's spec and the latest failure event, by
// pod, container and probe. cut counts the probes left out.
func probesOf(model inventory.Reader, p incident.Incident) ([]ProbeRow, int) {
	seen := map[probeKey]bool{}
	var out []ProbeRow
	for _, f := range p.Members {
		probe, ok := probeOfReason[f.Reason]
		if !ok || f.Advisory {
			continue
		}
		for _, c := range probeContainers(model, f.Entity, probe) {
			key := probeKey{c, probe}
			if seen[key] {
				continue
			}
			seen[key] = true
			if e, ok := model.Entity(c); ok {
				out = append(out, probeRow(model, e, probe))
			}
		}
	}
	slices.SortFunc(out, func(a, b ProbeRow) int {
		return cmp.Or(cmp.Compare(a.Namespace, b.Namespace),
			cmp.Compare(a.Pod, b.Pod), cmp.Compare(a.Container, b.Container),
			cmp.Compare(a.Probe, b.Probe))
	})
	if len(out) > maxProbes {
		return out[:maxProbes], len(out) - maxProbes
	}
	return out, 0
}

// probeContainers is the container a finding is about, or the
// containers of its pod that declare the probe.
func probeContainers(
	model inventory.Reader, id inventory.EntityID, probe string,
) []inventory.EntityID {
	if id.Kind == kube.KindContainer {
		return []inventory.EntityID{id}
	}
	if id.Kind != kube.KindPod {
		return nil
	}
	var out []inventory.EntityID
	for _, c := range containerIDs(model, id) {
		if _, ok := model.Attribute(c, kube.ProbeAttr(probe,
			"period")); ok {
			out = append(out, c)
		}
	}
	return out
}

// probeRow reads one probe's settings and its latest failure.
func probeRow(
	model inventory.Reader, e inventory.Entity, probe string,
) ProbeRow {
	pod, name, _ := strings.Cut(e.ID.Name, "/")
	setting := func(s string) int {
		return int(number(e, kube.ProbeAttr(probe, s)))
	}
	row := ProbeRow{Namespace: e.ID.Namespace, Pod: pod, Container: name,
		Probe: probe, Check: probeCheck(e, probe),
		Delay: setting("delay"), Timeout: setting("timeout"),
		Period: setting("period"), Failures: setting("failures"),
	}
	prefix := strings.ToUpper(probe[:1]) + probe[1:] + " probe"
	for _, note := range model.Notes(e.ID, time.Time{}) {
		if !note.Warning || note.Reason != unhealthyReason ||
			!strings.HasPrefix(note.Message, prefix) ||
			note.At.Before(row.LastAt) {
			continue
		}
		row.LastAt, row.Count = note.At, note.Count
		row.LastMessage = redact.Evidence(note.Message)
	}
	return row
}

// probeCheck is what the probe checks; an exec probe names only its
// program.
func probeCheck(e inventory.Entity, probe string) string {
	check := text(e, kube.ProbeAttr(probe, "check"))
	if check != "exec" {
		return redact.Evidence(check)
	}
	if program := text(e, kube.ProbeAttr(probe, "command")); program != "" {
		return "exec " + redact.Evidence(program)
	}
	return check
}
