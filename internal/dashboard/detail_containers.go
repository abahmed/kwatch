package dashboard

import (
	"slices"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/redact"
)

// maxContainers bounds the container rows kept, most restarted first.
const maxContainers = 10

// containersOf lists the containers of p's pods as the inventory has
// them now, most restarted first; cut counts the rows left out.
func containersOf(
	model inventory.HistoryReader, p incident.Incident,
) ([]ContainerRow, int) {
	states, cut := incident.ContainersOf(model, p.Root, memberIDs(p),
		maxContainers)
	return toContainerRows(states), cut
}

// toContainerRows serves container states, the image redacted again.
func toContainerRows(states []incident.ContainerState) []ContainerRow {
	rows := make([]ContainerRow, 0, len(states))
	for _, s := range states {
		rows = append(rows, ContainerRow{Namespace: s.Namespace,
			Pod: s.Pod, Container: s.Container, Ready: s.Ready,
			Restarts: s.Restarts, State: s.State,
			StateReason: s.StateReason, LastReason: s.LastReason,
			LastExitCode: s.LastExitCode, LastFinished: s.LastFinished,
			Image:      redact.Evidence(s.Image),
			MemoryUsed: s.MemoryUsed, MemoryReq: s.MemoryReq,
			MemoryLimit: s.MemoryLimit, CPUUsed: s.CPUUsed,
			CPUReq: s.CPUReq, CPULimit: s.CPULimit, UsageAt: s.UsageAt})
	}
	return rows
}

// podsOf is the pods of p's root and failing members, by name, at most
// incident.PodsOf's bound.
func podsOf(model inventory.Reader, p incident.Incident) []inventory.EntityID {
	return incident.PodsOf(model, p.Root, memberIDs(p))
}

// memberIDs are the objects of p's failure findings, by name.
func memberIDs(p incident.Incident) []inventory.EntityID {
	var out []inventory.EntityID
	for _, f := range p.Members {
		if !f.Advisory {
			out = append(out, f.Entity)
		}
	}
	slices.SortFunc(out, compareEntities)
	return out
}

func containerIDs(
	model inventory.Reader, pod inventory.EntityID,
) []inventory.EntityID {
	var out []inventory.EntityID
	for _, id := range model.Related(pod, inventory.PartOf,
		inventory.Incoming) {
		if id.Kind == kube.KindContainer {
			out = append(out, id)
		}
	}
	slices.SortFunc(out, compareEntities)
	return out
}

func compareEntities(a, b inventory.EntityID) int {
	return strings.Compare(a.String(), b.String())
}

func text(e inventory.Entity, name string) string {
	if a, ok := e.Attribute(name); ok {
		return a.Value.AsText()
	}
	return ""
}

func number(e inventory.Entity, name string) float64 {
	if a, ok := e.Attribute(name); ok {
		n, _ := a.Value.AsNumber()
		return n
	}
	return 0
}

func timeOf(e inventory.Entity, name string) time.Time {
	if a, ok := e.Attribute(name); ok {
		return a.Value.AsTime()
	}
	return time.Time{}
}
