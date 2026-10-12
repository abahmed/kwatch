package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// nodeDegradedRows cover a node that still serves but struggles: a
// kubelet that falls behind its pods, a container runtime that errors,
// a node whose tasks stall on CPU, memory or disk, a kubelet evicting
// pods, and any other node condition that is True. Weaker than a node
// that is down: the pods it hurts have many other causes, so the node
// needs a failing pod on it, and a node that is only strained never
// counts as broken (see view.broken).
var nodeDegradedRows = []Row{{
	Name: "node-degraded",
	Cause: Side{Kind: kube.KindNode, Modes: []detection.Mode{
		detection.ModePLEGSlow, detection.ModeRuntimeErrors,
		detection.ModePressureStall, detection.ModeEvicting,
		detection.ModeNodeCondition}},
	Link: LinkRunsOn,
	Effect: podSide(
		detection.ModeNotReady, detection.ModeProbe,
		detection.ModeCreating, detection.ModeCreateError,
		detection.ModeCrashLoop, detection.ModeEvicted),
	Prior: 0.55,
}}
