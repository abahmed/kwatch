package incident

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/redact"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// maxPods bounds the pods whose containers are read for one incident.
const maxPods = 32

// ContainerState is one container of an incident's pods as the
// inventory has it. The image is redacted; a state's message may hold
// secrets and is never read. Usage is in bytes and millicores, zero
// when unknown; UsageAt is when the kubelet last reported it.
type ContainerState struct {
	Namespace    string
	Pod          string
	Container    string
	Ready        *bool     `json:",omitempty"`
	Restarts     int       `json:",omitempty"`
	State        string    `json:",omitempty"`
	StateReason  string    `json:",omitempty"`
	LastReason   string    `json:",omitempty"`
	LastExitCode *int      `json:",omitempty"`
	LastFinished time.Time `json:",omitzero"`
	Image        string    `json:",omitempty"`
	MemoryUsed   int64     `json:",omitempty"`
	MemoryReq    int64     `json:",omitempty"`
	MemoryLimit  int64     `json:",omitempty"`
	CPUUsed      int64     `json:",omitempty"`
	CPUReq       int64     `json:",omitempty"`
	CPULimit     int64     `json:",omitempty"`
	UsageAt      time.Time `json:",omitzero"`
}

// ContainersOf lists the containers of the pods of root and members,
// most restarted first, at most limit; cut counts the ones left out.
func ContainersOf(
	model inventory.Reader, root inventory.EntityID,
	members []inventory.EntityID, limit int,
) (rows []ContainerState, cut int) {
	rows = readContainers(model, root, members)
	if len(rows) > limit {
		return rows[:limit], len(rows) - limit
	}
	return rows, 0
}

// readContainers reads every container of the pods of root and
// members, most restarted first.
func readContainers(
	model inventory.Reader, root inventory.EntityID,
	members []inventory.EntityID,
) []ContainerState {
	var rows []ContainerState
	for _, pod := range PodsOf(model, root, members) {
		for _, id := range containerIDs(model, pod) {
			if e, ok := model.Entity(id); ok {
				rows = append(rows, containerState(pod, e))
			}
		}
	}
	slices.SortFunc(rows, func(a, b ContainerState) int {
		return cmp.Or(cmp.Compare(b.Restarts, a.Restarts),
			cmp.Compare(a.Pod, b.Pod), cmp.Compare(a.Container, b.Container))
	})
	return rows
}

// PodsOf is the pods of root and members, at most maxPods: members' own
// pods first, then the pods a workload owns, each group by name, so a
// failing member pod is never cut for an owned one.
func PodsOf(
	model inventory.Reader, root inventory.EntityID,
	members []inventory.EntityID,
) []inventory.EntityID {
	seen := map[inventory.EntityID]bool{}
	var own, owned []inventory.EntityID
	add := func(list *[]inventory.EntityID, pod inventory.EntityID) {
		if !seen[pod] {
			seen[pod] = true
			*list = append(*list, pod)
		}
	}
	ids := append([]inventory.EntityID{root}, members...)
	for _, id := range ids {
		if pod, ok := rootcause.PodOf(model, id); ok {
			add(&own, pod)
		}
	}
	for _, id := range ids {
		if _, ok := rootcause.PodOf(model, id); ok {
			continue
		}
		for _, pod := range kube.OwnedPods(model, id) {
			add(&owned, pod)
		}
	}
	slices.SortFunc(own, compareIDs)
	slices.SortFunc(owned, compareIDs)
	pods := slices.Concat(own, owned)
	if len(pods) > maxPods {
		pods = pods[:maxPods]
	}
	return pods
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
	slices.SortFunc(out, compareIDs)
	return out
}

// containerState reads one container's attributes.
func containerState(
	pod inventory.EntityID, e inventory.Entity,
) ContainerState {
	_, name, _ := strings.Cut(e.ID.Name, "/")
	row := ContainerState{Namespace: pod.Namespace, Pod: pod.Name,
		Container:    name,
		Restarts:     int(attrNumber(e, kube.AttrRestarts)),
		State:        attrText(e, kube.AttrState),
		StateReason:  attrText(e, kube.AttrStateReason),
		LastReason:   attrText(e, kube.AttrLastReason),
		LastFinished: attrTime(e, kube.AttrLastFinished),
		Image:        redact.Evidence(attrText(e, kube.AttrImage)),
	}
	if a, ok := e.Attribute(kube.AttrReady); ok {
		if ready, isBool := a.Value.AsBool(); isBool {
			row.Ready = &ready
		}
	}
	if a, ok := e.Attribute(kube.AttrLastExitCode); ok {
		if code, isNumber := a.Value.AsNumber(); isNumber {
			c := int(code)
			row.LastExitCode = &c
		}
	}
	readUsage(&row, e)
	return row
}

// readUsage copies requests, limits and the kubelet's usage readings.
func readUsage(row *ContainerState, e inventory.Entity) {
	row.MemoryReq = int64(attrNumber(e, kube.AttrMemoryReq))
	row.MemoryLimit = int64(attrNumber(e, kube.AttrMemoryLimit))
	row.CPUReq = int64(attrNumber(e, kube.AttrCPUReq))
	row.CPULimit = int64(attrNumber(e, kube.AttrCPULimit))
	for _, name := range []string{kube.AttrMemoryWorking,
		kube.AttrCPUUsageMilli} {
		if a, ok := e.Attribute(name); ok && a.Updated.After(row.UsageAt) {
			row.UsageAt = a.Updated
		}
	}
	row.MemoryUsed = int64(attrNumber(e, kube.AttrMemoryWorking))
	row.CPUUsed = int64(attrNumber(e, kube.AttrCPUUsageMilli))
}

func attrText(e inventory.Entity, name string) string {
	if a, ok := e.Attribute(name); ok {
		return a.Value.AsText()
	}
	return ""
}

func attrTime(e inventory.Entity, name string) time.Time {
	if a, ok := e.Attribute(name); ok {
		return a.Value.AsTime()
	}
	return time.Time{}
}
