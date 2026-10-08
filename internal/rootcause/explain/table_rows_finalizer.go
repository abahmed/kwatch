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
	// No controller is known to be down: the objects that wait for the
	// same finalizer are one leftover. It needs two of them: one held
	// object is its own problem.
	Name: "finalizer-unhandled",
	Cause: Side{Kind: KindFinalizer,
		Modes: []detection.Mode{ModeFinalizerUnhandled}},
	Link: LinkFinalizes, Effect: stuckObject, Prior: 0.55,
	MinCovered: 2,
}})

// finalizerHandlerRows say that a controller with no running replica
// explains the objects whose finalizers it should remove. The controller
// is reached as the one that handles the finalizer, or, when the object
// names it as its field manager, as the one that manages it.
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
