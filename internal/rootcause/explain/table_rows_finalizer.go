package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
)

// stuckObject is an object, of any kind, whose deletion a finalizer
// holds.
var stuckObject = Side{Group: AnyGroup, Kind: AnyKind,
	Modes: []detection.Mode{detection.ModeStuckDeleting}}

// finalizerRows cover objects deleted long ago that stay because the
// controller that removes their finalizers does not run.
var finalizerRows = concatRows(finalizerHandlerRows(), []Row{{
	// No controller is known to handle the finalizer, or one that is
	// known runs: the objects that wait for it are one leftover. It
	// needs two of them: one held object is its own problem.
	Name: "finalizer-unhandled",
	Cause: Side{Kind: KindFinalizer,
		Modes: []detection.Mode{ModeFinalizerUnhandled}},
	Link: LinkFinalizes, Effect: stuckObject, Prior: 0.55,
	MinCovered: 2,
}})

// finalizerHandlerRows say that a controller with no running replica
// explains the objects whose finalizers it removes. Only evidence that
// it serves the finalizer's group makes it the handler (see
// handlersOf); the finalizes link reaches it. A walk keeps one link per
// target, so a handler that is also the object's field manager is reached
// by the manages link: the same evidence decides there too.
func finalizerHandlerRows() []Row {
	var rows []Row
	for _, link := range []LinkType{LinkFinalizes, LinkManages} {
		rows = append(rows, Row{
			Name: "finalizer-handler-stopped",
			Cause: Side{Group: AnyGroup, Kind: AnyKind,
				Modes: []detection.Mode{ModeHandlerStopped}},
			Link: link, Effect: stuckObject, Prior: 0.75,
		})
	}
	return rows
}
