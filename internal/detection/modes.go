package detection

import (
	"sort"
	"strings"
)

// Mode is a short, stable failure identity such as "CrashLoop" or
// "ImagePull.Registry". A dot separates a finer mode from its family:
// "ImagePull.Registry" is a kind of "ImagePull". Reasons without a
// shorter name are their own mode, so a custom detector can carry a mode
// that has no constant here.
type Mode string

// Within reports whether m is parent or a finer mode below it:
// "ImagePull.Registry" is within "ImagePull", "ImagePullX" is not.
func (m Mode) Within(parent Mode) bool {
	return m == parent || strings.HasPrefix(string(m), string(parent)+".")
}

// Family is the first segment of a mode: "CrashLoop.Panic" is a
// "CrashLoop".
func (m Mode) Family() Mode {
	family, _, _ := strings.Cut(string(m), ".")
	return Mode(family)
}

// ConditionMode prefixes the Mode of findings derived from one status
// condition, as in "Condition.Ready".
const ConditionMode = "Condition."

// ConditionModeOf is the mode of a finding derived from the status
// condition of the given type.
func ConditionModeOf(conditionType string) Mode {
	return Mode(ConditionMode + conditionType)
}

// Modes detectors set themselves, outside modeByReason.
const (
	// ModeMemoryEvictionThreshold and ModePIDEvictionThreshold name an
	// EvictionThresholdMet event by the resource the kubelet reclaims.
	ModeMemoryEvictionThreshold Mode = "Memory.EvictionThreshold"
	ModePIDEvictionThreshold    Mode = "PID.EvictionThreshold"
	// ModeFailedCreate is the mode of a FailedCreate event: the reason
	// has no shorter name, so it is its own mode.
	ModeFailedCreate Mode = "FailedCreate"
)

// Modes no built-in detector reports. A finding whose reason has no
// shorter name carries the reason as its mode, so custom detectors can
// report these, and propagation rows match them.
const (
	// ModeTerminating is an object stuck in its Terminating phase.
	// Built-in deletions are ModeStuckDeleting.
	ModeTerminating Mode = "Terminating"
	// ModeUnreachable is a pod on a node the control plane cannot
	// reach.
	ModeUnreachable Mode = "Unreachable"
)

// extraModes are the modes of the two blocks above.
var extraModes = []Mode{
	ModeMemoryEvictionThreshold, ModePIDEvictionThreshold,
	ModeFailedCreate, ModeTerminating, ModeUnreachable,
}

// Mode families that tables match as a whole. A family matches each of
// its finer modes: ModeExit matches ModeExitSegfault.
const (
	ModeDisk       Mode = "Disk"
	ModeExit       Mode = "Exit"
	ModeFilesystem Mode = "Filesystem"
	ModeInodes     Mode = "Inodes"
	ModeLatency    Mode = "Latency"
	ModeMemory     Mode = "Memory"
	ModeNetwork    Mode = "Network"
	ModeQuota      Mode = "Quota"
	ModeReference  Mode = "Reference"
	ModeVolume     Mode = "Volume"
	ModeWebhook    Mode = "Webhook"
)

// KnownMode reports whether some finding can carry m or a mode within
// m: a mode of modeByReason, one of extraModes, one of their families or
// a condition mode. Propagation rows are checked against it, so a typo
// in a table fails a test instead of silently matching nothing.
func KnownMode(m Mode) bool {
	if strings.HasPrefix(string(m), ConditionMode) {
		return true
	}
	for _, mode := range allModes() {
		if mode.Within(m) {
			return true
		}
	}
	return false
}

// allModes lists every mode of modeByReason and extraModes.
func allModes() []Mode {
	out := append([]Mode(nil), extraModes...)
	for _, mode := range modeByReason {
		out = append(out, mode)
	}
	return out
}

// ModeEntry is one failure mode and the reasons that map to it.
type ModeEntry struct {
	Mode    Mode
	Reasons []string
}

// Modes returns the failure-mode table, sorted by mode, with each mode's
// reasons sorted. It is a detached copy, for coverage checks and
// generated documentation.
func Modes() []ModeEntry {
	byMode := make(map[Mode][]string)
	for reason, mode := range modeByReason {
		byMode[mode] = append(byMode[mode], reason)
	}
	out := make([]ModeEntry, 0, len(byMode))
	for mode, rs := range byMode {
		sort.Strings(rs)
		out = append(out, ModeEntry{Mode: mode, Reasons: rs})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Mode < out[j].Mode })
	return out
}
