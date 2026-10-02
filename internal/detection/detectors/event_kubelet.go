package detectors

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
)

// Pod network setup event reasons (pkg/kubelet/events).
const (
	failedCreatePodSandBox = "FailedCreatePodSandBox"
	networkNotReady        = "NetworkNotReady"
)

// kubeletEventSummaries describe the kubelet's node events.
var kubeletEventSummaries = map[string]string{
	reasons.EvictionThresholdMet: "Node crossed an eviction threshold; " +
		"the kubelet is reclaiming resources",
	reasons.ImageGCFailed:       "Kubelet cannot garbage-collect images",
	reasons.FreeDiskSpaceFailed: "Kubelet cannot free enough disk space",
	reasons.ContainerGCFailed: "Kubelet cannot garbage-collect dead " +
		"containers",
}

// evictionResourceModes name EvictionThresholdMet after the resource in
// its message, "Attempting to reclaim <resource>" (kubelet eviction
// manager). Disk resources keep the reason's own Disk mode.
var evictionResourceModes = []struct {
	resource string
	mode     detection.Mode
}{
	{"memory", detection.ModeMemoryEvictionThreshold},
	{"pids", detection.ModePIDEvictionThreshold},
}

// sandboxClasses are pod network setup failure classes, matched in lower
// case against the sandbox error the runtime and CNI plugin return.
var sandboxClasses = []struct {
	mode     detection.Mode
	patterns []string
}{
	{detection.ModeNetworkIPExhausted, []string{
		"no ip addresses available", "failed to allocate for range",
		"failed to assign an ip address",
	}},
	{detection.ModeNetworkCNINotReady, []string{
		"network plugin is not ready", "cni plugin not initialized",
		"cni config uninitialized", "networkpluginnotready",
	}},
}

// eventMode names the failure mode an event's message makes more precise
// than its reason; "" keeps the reason's mode.
func eventMode(note inventory.Note) detection.Mode {
	switch note.Reason {
	case failedCreatePodSandBox, networkNotReady:
		return sandboxMode(note.Message)
	case reasons.EvictionThresholdMet:
		for _, r := range evictionResourceModes {
			if strings.HasSuffix(note.Message, "reclaim "+r.resource) {
				return r.mode
			}
		}
	}
	return ""
}

// sandboxMode classifies a pod network setup failure message.
func sandboxMode(message string) detection.Mode {
	lower := strings.ToLower(message)
	for _, class := range sandboxClasses {
		for _, pattern := range class.patterns {
			if strings.Contains(lower, pattern) {
				return class.mode
			}
		}
	}
	return ""
}

// sandboxFailure reports whether a note is a classified pod network setup
// failure and returns its mode.
func sandboxFailure(note inventory.Note) (detection.Mode, bool) {
	if !note.Warning || (note.Reason != failedCreatePodSandBox &&
		note.Reason != networkNotReady) {
		return "", false
	}
	mode := sandboxMode(note.Message)
	return mode, mode != ""
}
