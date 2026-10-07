package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
)

// Pseudo modes of a workload whose liveness probe kills its containers
// while they are still starting. They are read from the evidence of the
// container's own liveness finding, which the detector writes only for
// a run that ended before the container was ready.
const (
	// ModeLivenessStartShort: liveness allows less time than the
	// workload's own past starts needed.
	ModeLivenessStartShort detection.Mode = "Config.LivenessStartShort"
	// ModeLivenessNeverReady: the container never reached Ready before
	// a liveness kill, and no history says how long a start takes.
	ModeLivenessNeverReady detection.Mode = "Config.LivenessNeverReady"
)

// livenessStartRows cover a start that liveness cuts short.
var livenessStartRows = []Row{
	{
		// The history is the proof: starts took longer than liveness
		// allows, so every new container is killed before it is up.
		Name: "liveness-shorter-than-start",
		Cause: Side{Group: AnyGroup, Kind: AnyKind,
			Modes: []detection.Mode{ModeLivenessStartShort}},
		Link: LinkSelf,
		Effect: podSide(
			detection.ModeCrashLoop, detection.ModeRestarting,
			detection.ModeProbe, detection.ModeNotReady),
		Prior: 0.8, Inside: true,
	},
	{
		// Without history the fact is thin: a container that never got
		// ready before each kill may also hang. It stays below the rows
		// that have more to go on.
		Name: "liveness-kills-before-ready",
		Cause: Side{Group: AnyGroup, Kind: AnyKind,
			Modes: []detection.Mode{ModeLivenessNeverReady}},
		Link: LinkSelf,
		Effect: podSide(
			detection.ModeCrashLoop, detection.ModeRestarting,
			detection.ModeProbe, detection.ModeNotReady),
		Prior: 0.45, Inside: true,
	},
}

// livenessStartMode is the mode the liveness findings of the given
// entities carry evidence for, if any. History wins over the bare fact.
func (v *view) livenessStartMode(
	ids ...inventory.EntityID,
) (detection.Mode, bool) {
	var never bool
	for _, id := range ids {
		for _, f := range v.s.Findings[id] {
			for _, e := range f.Evidence {
				switch e.Label {
				case detection.EvidenceUsualStart:
					return ModeLivenessStartShort, true
				case detection.EvidenceKilledBeforeReady:
					never = true
				}
			}
		}
	}
	return ModeLivenessNeverReady, never
}
