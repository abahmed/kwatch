package explain

import (
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// Pseudo modes of a workload whose own configuration makes it fail.
// They are read from the failing container and its siblings, and only
// on the LinkSelf side of a row.
const (
	// ModeMemoryTooLow is a limit the container reaches soon after
	// every start: its steady need is above the limit. A leak runs
	// for a long time before it reaches the same limit.
	ModeMemoryTooLow detection.Mode = "Config.MemoryLimitTooLow"
	// ModeProbePortMismatch is a probe aimed at a port the container
	// does not declare.
	ModeProbePortMismatch detection.Mode = "Config.ProbePortMismatch"
	// ModeStartupBudgetShort is a startup or readiness probe budget
	// shorter than the time the workload's ready replicas needed.
	ModeStartupBudgetShort detection.Mode = "Config.StartupBudgetShort"
)

// workloadConfigRows cover a workload broken by its own settings. The
// cause is the workload; the evidence is the container's own record.
var workloadConfigRows = []Row{
	{
		Name: "memory-limit-too-low",
		Cause: Side{Group: AnyGroup, Kind: AnyKind,
			Modes: []detection.Mode{ModeMemoryTooLow}},
		Link: LinkSelf, Effect: podSide(detection.ModeOOMKilled),
		Prior: 0.7, Inside: true,
	},
	{
		Name: "probe-port-mismatch",
		Cause: Side{Group: AnyGroup, Kind: AnyKind,
			Modes: []detection.Mode{ModeProbePortMismatch}},
		Link: LinkSelf,
		Effect: podSide(
			detection.ModeProbe, detection.ModeNotReady,
			detection.ModeCrashLoop, detection.ModeRestarting),
		Prior: 0.8, Inside: true,
	},
	{
		Name: "startup-budget-too-short",
		Cause: Side{Group: AnyGroup, Kind: AnyKind,
			Modes: []detection.Mode{ModeStartupBudgetShort}},
		Link: LinkSelf,
		Effect: podSide(
			detection.ModeProbe, detection.ModeCrashLoop,
			detection.ModeRestarting, detection.ModeNotReady),
		Prior: 0.75, Inside: true,
	},
}

// selfConfigModes are the pseudo modes of effect's own configuration.
// A pod's modes are those of its containers: a pod is unready because
// of the probe of one of them.
func (v *view) selfConfigModes(effect inventory.EntityID) []modeHealth {
	containers := []inventory.EntityID{effect}
	if effect.Kind == kube.KindPod {
		containers = v.s.Model.Related(effect, inventory.PartOf,
			inventory.Incoming)
	}
	seen := map[detection.Mode]bool{}
	var out []modeHealth
	for _, id := range containers {
		for _, mode := range v.containerConfigModes(id, effect) {
			if !seen[mode] {
				seen[mode] = true
				out = append(out, modeHealth{mode: mode,
					health: detection.Failing, pseudo: true})
			}
		}
	}
	return out
}

// containerConfigModes are the configuration faults one container
// shows; failing is the entity whose findings say how it fails (the
// container itself, or its pod).
func (v *view) containerConfigModes(
	id, failing inventory.EntityID,
) []detection.Mode {
	container, ok := v.s.Model.Entity(id)
	if !ok || id.Kind != kube.KindContainer {
		return nil
	}
	var out []detection.Mode
	if v.hasMode(failing, []detection.Mode{detection.ModeOOMKilled}) &&
		shortLastRun(container) {
		out = append(out, ModeMemoryTooLow)
	}
	if probeMissesPorts(container) {
		out = append(out, ModeProbePortMismatch)
	}
	if v.startupBudgetShort(failing, container) {
		out = append(out, ModeStartupBudgetShort)
	}
	return out
}

// shortLastRun reports a last run that ended within OOMSteadyRun of
// its start.
func shortLastRun(container inventory.Entity) bool {
	started, ok1 := attrTime(container, kube.AttrLastStarted)
	finished, ok2 := attrTime(container, kube.AttrLastFinished)
	return ok1 && ok2 && finished.Sub(started) <= OOMSteadyRun
}

// probeMissesPorts reports a probe port the container does not declare.
// A container that declares no ports says nothing: declaring them is
// optional.
func probeMissesPorts(container inventory.Entity) bool {
	declared := attrText(container, kube.AttrContainerPorts)
	probed := attrText(container, kube.AttrProbePorts)
	if declared == "" || probed == "" {
		return false
	}
	ports := map[string]bool{}
	for _, port := range strings.Split(declared, ",") {
		ports[port] = true
	}
	for _, port := range strings.Split(probed, ",") {
		if !ports[port] {
			return true
		}
	}
	return false
}

// startupBudgetShort reports a failing container whose probe budget is
// shorter than the startup the same workload's ready pods needed.
func (v *view) startupBudgetShort(
	effect inventory.EntityID, container inventory.Entity,
) bool {
	budget, ok := attrNumber(container, kube.AttrProbeBudget)
	if !ok || !v.hasMode(effect, []detection.Mode{
		detection.ModeProbe, detection.ModeCrashLoop}) {
		return false
	}
	needed, ok := v.readyStartup(effect)
	return ok && time.Duration(budget)*time.Second < needed
}

// readyStartup is the longest time a ready pod of effect's workload
// took from its start to ready: what the workload needs to start.
func (v *view) readyStartup(effect inventory.EntityID) (time.Duration, bool) {
	pod, ok := v.podOf(effect)
	if !ok {
		return 0, false
	}
	var longest time.Duration
	for _, sibling := range v.workloadPods(pod) {
		if sibling == pod || v.unitFailing(sibling) {
			continue
		}
		if took, ok := startupOf(v.s.Model, sibling); ok && took > longest {
			longest = took
		}
	}
	return longest, longest > 0
}

// workloadPods lists the pods of pod's top owner: its own ReplicaSet's
// pods and those of the owner's other ReplicaSets, sampled.
func (v *view) workloadPods(pod inventory.EntityID) []inventory.EntityID {
	root := rootcause.TopOwner(v.s.Model, pod)
	var out []inventory.EntityID
	frontier := []inventory.EntityID{root}
	for depth := 0; depth < 2 && len(frontier) > 0; depth++ {
		var next []inventory.EntityID
		for _, owner := range frontier {
			for _, child := range v.s.Model.Related(owner,
				inventory.OwnedBy, inventory.Incoming) {
				if child.Kind == kube.KindPod {
					out = append(out, child)
				} else {
					next = append(next, child)
				}
			}
		}
		frontier = next
	}
	return sampleIDs(out)
}

// startupOf is how long a ready pod took from its start to ready.
func startupOf(model inventory.Reader, pod inventory.EntityID) (
	time.Duration, bool,
) {
	e, ok := model.Entity(pod)
	if !ok {
		return 0, false
	}
	ready, _ := e.Attribute(kube.AttrReady)
	if isReady, _ := ready.Value.AsBool(); !isReady {
		return 0, false
	}
	started, ok1 := attrTime(e, kube.AttrStartTime)
	since, ok2 := attrTime(e, kube.AttrReadySince)
	if !ok1 || !ok2 || since.Before(started) {
		return 0, false
	}
	return since.Sub(started), true
}

func attrText(e inventory.Entity, name string) string {
	attribute, ok := e.Attribute(name)
	if !ok {
		return ""
	}
	return attribute.Value.AsText()
}

func attrTime(e inventory.Entity, name string) (time.Time, bool) {
	attribute, ok := e.Attribute(name)
	if !ok {
		return time.Time{}, false
	}
	t := attribute.Value.AsTime()
	return t, !t.IsZero()
}

func attrNumber(e inventory.Entity, name string) (float64, bool) {
	attribute, ok := e.Attribute(name)
	if !ok {
		return 0, false
	}
	return attribute.Value.AsNumber()
}
