package investigate

import (
	"context"
	"strings"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// labelled is one attribute shown under a label.
type labelled struct{ label, attr string }

// podFacts and containerFacts are what "kubectl describe pod" shows
// that the model holds.
var (
	podFacts = []labelled{
		{"Phase", kube.AttrPhase}, {"Reason", kube.AttrReason},
		{"QoS", kube.AttrQoS}, {"Message", kube.AttrMessage},
	}
	containerFacts = []labelled{
		{"State", kube.AttrState}, {"Waiting reason", kube.AttrStateReason},
		{"Restarts", kube.AttrRestarts},
		{"Last termination", kube.AttrLastReason},
		{"Last exit code", kube.AttrLastExitCode},
		{"Last message", kube.AttrLastMessage},
		{"Image", kube.AttrImage},
		{"Memory request", kube.AttrMemoryReq},
		{"Memory limit", kube.AttrMemoryLimit},
		{"CPU request", kube.AttrCPUReq}, {"CPU limit", kube.AttrCPULimit},
	}
)

// hasPods reports an incident with pods to read.
func hasPods(p incident.Incident, model inventory.Reader) bool {
	return len(incidentPods(model, p)) > 0
}

// readPodState reads each pod's phase, conditions, node and, per
// container, state, last termination, restarts and resources.
func readPodState(
	_ context.Context, s Sources, p incident.Incident,
) []incident.Section {
	var out []incident.Section
	for _, pod := range incidentPods(s.Model, p) {
		e, ok := s.Model.Entity(pod)
		if !ok {
			continue
		}
		sec := newSection(pod.Name)
		addFacts(sec, "", e, podFacts)
		for _, node := range relatedOf(s.Model, pod, inventory.RunsOn,
			inventory.Outgoing, kube.KindNode) {
			sec.fact("Node", node.Name)
		}
		for _, c := range failingConditions(e) {
			sec.fact("Condition", c)
		}
		for _, c := range containersOf(s.Model, pod) {
			if ce, ok := s.Model.Entity(c); ok {
				_, name := splitName(c.Name)
				addFacts(sec, name+": ", ce, containerFacts)
			}
		}
		out = append(out, sec.done())
	}
	return out
}

func addFacts(
	sec *section, prefix string, e inventory.Entity, facts []labelled,
) {
	for _, f := range facts {
		sec.fact(prefix+f.label, textOf(e, f.attr))
	}
}

func splitName(name string) (string, string) {
	pod, container, _ := strings.Cut(name, "/")
	return pod, container
}

// readLogs reads every container's previous run, when it restarted,
// and current run. Pods whose output has the signature of one already
// read are skipped, so replicas that fail alike are shown once.
func readLogs(
	ctx context.Context, s Sources, p incident.Incident,
) []incident.Section {
	if s.LogsOff {
		return []incident.Section{{Status: incident.SectionDenied,
			Reason: incident.ReasonOff}}
	}
	if s.Tail == nil {
		return []incident.Section{{Status: incident.SectionDenied,
			Reason: incident.ReasonRBAC}}
	}
	var out []incident.Section
	seen := map[string]bool{}
	for _, pod := range incidentPods(s.Model, p) {
		var pending []incident.Section
		fresh := false
		for _, c := range containersOf(s.Model, pod) {
			for _, previous := range runsOf(s.Model, c) {
				sec := readTail(ctx, s, c, previous)
				pending = append(pending, sec)
				sig := sectionSignature(sec)
				fresh = fresh || sig == "" || !seen[sig]
				seen[sig] = true
			}
		}
		if fresh {
			out = append(out, pending...)
		}
	}
	return out
}

// runsOf lists the runs to read: the previous one when the container
// ran before, then the current one.
func runsOf(model inventory.Reader, c inventory.EntityID) []bool {
	e, _ := model.Entity(c)
	if textOf(e, kube.AttrLastReason) != "" ||
		(textOf(e, kube.AttrRestarts) != "" &&
			textOf(e, kube.AttrRestarts) != "0") {
		return []bool{true, false}
	}
	return []bool{false}
}

// readTail reads one run into a section; error-looking lines are
// matched.
func readTail(
	ctx context.Context, s Sources, c inventory.EntityID, previous bool,
) incident.Section {
	sec := newSection(c.Name)
	sec.Run = "current"
	if previous {
		sec.Run = "previous"
	}
	lines, err := s.Tail(ctx, c, previous)
	if status, reason := readStatus(ctx, err); err != nil {
		sec.Status, sec.Reason = status, reason
		return sec.done()
	}
	for _, line := range lines {
		sec.line(line, kube.IsErrorLine(line))
	}
	return sec.done()
}

// sectionSignature is the signature of a section's first error line.
func sectionSignature(s incident.Section) string {
	if line := kube.FirstErrorLine(s.Lines); line != "" {
		return logSignature(line)
	}
	return ""
}

// readEvents lists the events of the pods, their owner, node and
// volume claims and of the root, newest first.
func readEvents(
	_ context.Context, s Sources, p incident.Incident,
) []incident.Section {
	ids := []inventory.EntityID{p.Root}
	for _, pod := range incidentPods(s.Model, p) {
		ids = append(ids, pod)
		ids = append(ids, containersOf(s.Model, pod)...)
		ids = append(ids, s.Model.Related(pod, inventory.OwnedBy,
			inventory.Outgoing)...)
		ids = append(ids, s.Model.Related(pod, inventory.RunsOn,
			inventory.Outgoing)...)
		ids = append(ids, s.Model.Related(pod, inventory.Mounts,
			inventory.Outgoing)...)
	}
	sec := newSection("")
	for _, line := range notesOf(s.Model, p, ids) {
		sec.line(line, false)
	}
	return []incident.Section{sec.done()}
}

// usesMemoryOrCPU reports an incident whose containers' use matters: a
// memory kill, CPU throttling or any container member.
func usesMemoryOrCPU(p incident.Incident, model inventory.Reader) bool {
	return hasMode(p, []string{"OOMKilled", "CPUThrottled", "CrashLoop",
		"Restarting"}) && hasPods(p, model)
}

// readUsage compares each container's use with its request and limit,
// from the kubelet stats kwatch already polls (nodes/stats): the current
// working set and CPU, the 24h peak and the run before the last kill. It
// needs no add-on such as metrics-server.
func readUsage(
	_ context.Context, s Sources, p incident.Incident,
) []incident.Section {
	var out []incident.Section
	for _, pod := range incidentPods(s.Model, p) {
		sec := newSection(pod.Name)
		for _, c := range containersOf(s.Model, pod) {
			addUsage(sec, s.Model, c)
		}
		out = append(out, sec.done())
	}
	return out
}

// addUsage adds one container's use and limits.
func addUsage(sec *section, model inventory.Reader, c inventory.EntityID) {
	e, ok := model.Entity(c)
	if !ok {
		return
	}
	_, name := splitName(c.Name)
	memory, cpu := number(e, kube.AttrMemoryWorking),
		number(e, kube.AttrCPUUsageMilli)
	if memory > 0 {
		sec.fact(name+": memory", humanMemory(memory)+limitOf(e,
			kube.AttrMemoryLimit))
	}
	if cpu > 0 {
		sec.fact(name+": CPU", humanCPU(cpu)+limitOf(e, kube.AttrCPULimit))
	}
	sec.fact(name+": memory request", textOf(e, kube.AttrMemoryReq))
	sec.fact(name+": memory limit", textOf(e, kube.AttrMemoryLimit))
	if peak := number(e, kube.AttrMemoryPeak24h); peak > 0 {
		sec.fact(name+": memory peak, last 24h", humanMemory(peak))
	}
	if peak := number(e, kube.AttrMemoryPrevPeak); peak > 0 {
		sec.fact(name+": memory before the last kill",
			"peak "+humanMemory(peak)+", from "+
				humanMemory(number(e, kube.AttrMemoryPrevStart)))
	}
}

func limitOf(e inventory.Entity, attr string) string {
	if limit := textOf(e, attr); limit != "" {
		return " (limit " + limit + ")"
	}
	return ""
}

// probeModes are the failure modes a probe check explains.
var probeModes = []string{"Probe", "Liveness", "Readiness", "Unready"}

func failsProbe(p incident.Incident, _ inventory.Reader) bool {
	return hasMode(p, probeModes)
}

// probeFacts are a container's probe settings and last failure.
var probeFacts = []labelled{
	{"Probes", kube.AttrProbes}, {"Last failure", kube.AttrProbeError},
	{"Failure kind", kube.AttrProbeFailureKind},
	{"Liveness failures", kube.AttrLivenessFailures},
	{"Timeout seconds", kube.AttrProbeTimeoutSeconds},
}

// readProbes reads each container's probe spec and last failure, and
// the probe events of its pod.
func readProbes(
	_ context.Context, s Sources, p incident.Incident,
) []incident.Section {
	var out []incident.Section
	for _, pod := range incidentPods(s.Model, p) {
		sec := newSection(pod.Name)
		for _, c := range containersOf(s.Model, pod) {
			if e, ok := s.Model.Entity(c); ok {
				_, name := splitName(c.Name)
				addFacts(sec, name+": ", e, probeFacts)
			}
		}
		for _, line := range notesOf(s.Model, p,
			[]inventory.EntityID{pod}) {
			if strings.Contains(line, "Unhealthy") {
				sec.line(line, true)
			}
		}
		out = append(out, sec.done())
	}
	return out
}
