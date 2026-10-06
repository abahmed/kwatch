package investigate

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// maxTopPods bounds the heaviest pods named per resource.
const maxTopPods = 3

// pressureConditions are node conditions where True is the problem.
var pressureConditions = []string{
	"MemoryPressure", "DiskPressure", "PIDPressure", "NetworkUnavailable",
}

// readNode reads the root node's failing conditions and its heaviest
// pods by memory and CPU from the latest kubelet stats in the model. It
// makes no API call.
func readNode(_ context.Context, s Sources, p incident.Incident) Result {
	node, ok := s.Model.Entity(p.Root)
	if !ok {
		return Result{}
	}
	var r Result
	if failing := nodeConditions(node); failing != "" {
		r.Evidence = append(r.Evidence,
			incident.Fact{Kind: incident.FactNode, Text: failing})
	}
	usage := podUsageOn(s.Model, p.Root)
	if top := topPods(usage, func(u podUsage) float64 { return u.memory },
		humanMemory); top != "" {
		r.Evidence = append(r.Evidence,
			incident.Fact{Kind: incident.FactTopMemory, Text: top})
	}
	if top := topPods(usage, func(u podUsage) float64 { return u.cpu },
		humanCPU); top != "" {
		r.Evidence = append(r.Evidence,
			incident.Fact{Kind: incident.FactTopCPU, Text: top})
	}
	return r
}

// nodeConditions lists the failing conditions of a node: "Ready=False
// (KubeletNotReady), MemoryPressure".
func nodeConditions(node inventory.Entity) string {
	var failing []string
	if status := conditionText(node, "Ready", ""); status != "" &&
		status != "True" {
		text := "Ready=" + status
		if reason := conditionText(node, "Ready",
			kube.AttrConditionReason); reason != "" {
			text += " (" + reason + ")"
		}
		failing = append(failing, text)
	}
	for _, c := range pressureConditions {
		if conditionText(node, c, "") == "True" {
			failing = append(failing, c)
		}
	}
	return strings.Join(failing, ", ")
}

func conditionText(e inventory.Entity, condition, suffix string) string {
	a, ok := e.Attribute(kube.ConditionKey(condition) + suffix)
	if !ok {
		return ""
	}
	return a.Value.AsText()
}

// podUsage is one pod's summed container usage.
type podUsage struct {
	name   string
	memory float64
	cpu    float64
}

// podUsageOn sums the latest container usage of every pod on node.
func podUsageOn(model inventory.Reader, node inventory.EntityID) []podUsage {
	var out []podUsage
	for _, pod := range model.Related(node, inventory.RunsOn,
		inventory.Incoming) {
		u := podUsage{name: pod.Name}
		for _, c := range model.Related(pod, inventory.PartOf,
			inventory.Incoming) {
			container, ok := model.Entity(c)
			if !ok {
				continue
			}
			u.memory += number(container, kube.AttrMemoryWorking)
			u.cpu += number(container, kube.AttrCPUUsageMilli)
		}
		out = append(out, u)
	}
	return out
}

func number(e inventory.Entity, name string) float64 {
	a, ok := e.Attribute(name)
	if !ok {
		return 0
	}
	n, _ := a.Value.AsNumber()
	return n
}

// topPods names the heaviest pods by one resource: "etl-0 (3.1 GiB),
// api-7f (1.2 GiB)". Pods without usage are left out.
func topPods(
	usage []podUsage, of func(podUsage) float64, format func(float64) string,
) string {
	sorted := append([]podUsage(nil), usage...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if of(sorted[i]) != of(sorted[j]) {
			return of(sorted[i]) > of(sorted[j])
		}
		return sorted[i].name < sorted[j].name
	})
	var names []string
	for _, u := range sorted {
		if of(u) <= 0 || len(names) == maxTopPods {
			break
		}
		names = append(names, u.name+" ("+format(of(u))+")")
	}
	return strings.Join(names, ", ")
}

func humanMemory(bytes float64) string {
	const gib, mib = 1 << 30, 1 << 20
	if bytes >= gib {
		return fmt.Sprintf("%.1f GiB", bytes/gib)
	}
	return fmt.Sprintf("%.0f MiB", bytes/mib)
}

// milliPerCore converts CPU millicores to cores.
const milliPerCore = 1000

func humanCPU(milli float64) string {
	if milli >= milliPerCore {
		return fmt.Sprintf("%.1f cores", milli/milliPerCore)
	}
	return fmt.Sprintf("%.0fm CPU", milli)
}
