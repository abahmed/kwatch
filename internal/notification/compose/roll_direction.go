package compose

import (
	"strconv"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// Which way a fixing change moved a workload. Revision numbers alone
// do not say it: `kubectl rollout undo` and a GitOps revert give the
// template they bring back the next Deployment revision (the highest
// plus one), and DaemonSet and StatefulSet revisions grow on a rollback
// too. So the content decides first: the fix sets the fields the blamed
// change set back to the values they had before it, or the incident's
// history found the blamed change reverted (incident.FixRollback, from
// inventory.OutcomeReverted). Only when the content cannot tell do the
// numbers: a revision below the blamed one is a return, one above a new
// release, and the revision a rollback would return to is a return.

// rollWay is the way a fixing change moved a workload.
type rollWay int

const (
	rollUnknown rollWay = iota
	rollBack
	rollForward
)

// rollDirection is the way fix moved the workload cause blames, given
// how the incident ended.
func rollDirection(
	fix *inventory.Change, cause *rootcause.CauseRecord, ended incident.Fix,
) rollWay {
	if fix == nil || cause == nil {
		return rollUnknown
	}
	if back, known := fieldsReturned(fix, cause.Change); known {
		if back {
			return rollBack
		}
		return rollForward
	}
	if ended == incident.FixRollback {
		return rollBack
	}
	if cause.Change != nil {
		order, ok := revisionOrder(fix.Revision, cause.Change.Revision)
		switch {
		case ok && order < 0:
			return rollBack
		case ok && order > 0:
			return rollForward
		case ok:
			return rollUnknown
		}
	}
	if fix.Revision != "" && cause.RollbackRevision == fix.Revision {
		return rollBack
	}
	return rollUnknown
}

// fieldsReturned compares the fields fix set with the ones blamed
// changed: back when each it set again is at its value from before
// blamed. known is false when fix set none of them, or blamed recorded
// no old value to compare with.
func fieldsReturned(fix, blamed *inventory.Change) (back, known bool) {
	if blamed == nil {
		return false, false
	}
	after := make(map[string]string, len(fix.Fields))
	for _, field := range fix.Fields {
		after[field.Path] = field.After
	}
	for _, field := range blamed.Fields {
		if field.Before == "" || field.Before == field.After {
			continue
		}
		now, ok := after[field.Path]
		if !ok {
			continue
		}
		if now != field.Before {
			return false, true
		}
		back = true
	}
	return back, back
}

// rolledBackWords say a rollback: "rolled back to revision 13". A
// rollback whose revision number went up (a rollout undo gives the
// template it brings back the next number) says so, so the number does
// not read as a new release.
func rolledBackWords(
	fix *inventory.Change, cause *rootcause.CauseRecord,
) string {
	if cause != nil && cause.Change != nil {
		if order, ok := revisionOrder(fix.Revision,
			cause.Change.Revision); ok && order > 0 {
			return "rolled back to the earlier version (now revision " +
				fix.Revision + ")"
		}
	}
	return "rolled back to revision " + fix.Revision
}

// revisionOrder compares two numeric revisions: -1, 0 or 1 as a is
// below, at or above b. false when either is not a number.
func revisionOrder(a, b string) (int, bool) {
	x, errA := strconv.ParseInt(a, 10, 64)
	y, errB := strconv.ParseInt(b, 10, 64)
	switch {
	case errA != nil || errB != nil:
		return 0, false
	case x < y:
		return -1, true
	case x > y:
		return 1, true
	}
	return 0, true
}
