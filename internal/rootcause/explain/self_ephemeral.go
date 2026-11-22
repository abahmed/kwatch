package explain

import (
	"regexp"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// ModeEphemeralTooLow is a pod the kubelet evicted for using more
// local ephemeral storage than its own containers' limits allow. It is
// a pseudo mode on the LinkSelf side of a row, like ModeMemoryTooLow.
const ModeEphemeralTooLow detection.Mode = "Config.EphemeralLimitTooLow"

// ephemeralOverLimit matches the kubelet's eviction messages for a pod
// or a container over its own ephemeral-storage limit. Pressure on the
// node's disk words its evictions differently ("The node was low on
// resource: ephemeral-storage") and does not match.
var ephemeralOverLimit = regexp.MustCompile(
	`ephemeral local storage usage exceeds the total limit|` +
		`exceeded its local ephemeral storage limit`)

// ephemeralConfigRow blames the workload whose pods outgrow their own
// ephemeral-storage limit.
var ephemeralConfigRow = Row{
	Name: "ephemeral-limit-too-low",
	Cause: Side{Group: AnyGroup, Kind: AnyKind,
		Modes: []detection.Mode{ModeEphemeralTooLow}},
	Link:   LinkSelf,
	Effect: podSide(detection.ModeEvicted, detection.ModeFailed),
	Prior:  0.75, Inside: true,
}

// podConfigModes are the configuration faults a pod's own record
// shows: its eviction message says its ephemeral-storage limit was hit.
func (v *view) podConfigModes(pod inventory.EntityID) []detection.Mode {
	modes := v.pullConfigModes(pod)
	if pod.Kind != kube.KindPod ||
		!v.hasMode(pod, []detection.Mode{detection.ModeEvicted,
			detection.ModeFailed}) {
		return modes
	}
	text := strings.ToLower(v.text(pod))
	if ephemeralOverLimit.MatchString(text) {
		modes = append(modes, ModeEphemeralTooLow)
	}
	if memoryOverRequest.MatchString(text) {
		modes = append(modes, ModeMemoryOverRequest)
	}
	return modes
}

// ModeMemoryOverRequest is a pod the kubelet evicted for memory, naming
// the container that used more than it requests: the node was short, but
// the kubelet picked this pod because of its own use.
const ModeMemoryOverRequest detection.Mode = "Config.MemoryOverRequest"

// memoryOverRequest matches the kubelet's eviction message for a
// container whose use is above its memory request.
var memoryOverRequest = regexp.MustCompile(
	`was using [^,]+, request is [^,]+, has larger consumption of memory`)

// memoryOverRequestRow blames the workload the kubelet named as using
// more memory than it requests, instead of the node that was short.
var memoryOverRequestRow = Row{
	Name: "memory-over-request",
	Cause: Side{Group: AnyGroup, Kind: AnyKind,
		Modes: []detection.Mode{ModeMemoryOverRequest}},
	Link:   LinkSelf,
	Effect: podSide(detection.ModeEvicted, detection.ModeFailed),
	Prior:  0.9, Inside: true,
}
