package compose

import (
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// revisionEdits are what the failing revision changed from the healthy
// one, from the cause's revision proof, or else from a release
// regression's own evidence. The number is the revision.
func revisionEdits(f caseFacts) (int, []inventory.FieldChange) {
	if f.p.Cause != nil {
		for _, item := range f.p.Cause.Proof {
			if item.Code == rootcause.ProofNewRevisionFails &&
				item.Supports && len(item.Edits) > 0 {
				return item.Count, item.Edits
			}
		}
	}
	if own := releaseRegression(f); own != nil {
		return evidenceEdits(*own)
	}
	return 0, nil
}

// evidenceEdits reads the edits a release regression finding lists.
func evidenceEdits(own detection.Finding) (int, []inventory.FieldChange) {
	revision, _ := strconv.Atoi(evidence(own, "new revision"))
	var edits []inventory.FieldChange
	for _, item := range own.Evidence {
		path, ok := strings.CutPrefix(item.Label, detection.EvidenceEditPrefix)
		if !ok {
			continue
		}
		before, after, _ := strings.Cut(item.Value, detection.EvidenceEditArrow)
		edits = append(edits, inventory.FieldChange{Path: path,
			Before: setIfUnset(before), After: setIfUnset(after)})
	}
	return revision, edits
}

func setIfUnset(value string) string {
	if value == "unset" {
		return ""
	}
	return value
}

// revisionDiffSentences say what the failing revision changed: "Only
// change in revision 14: memory limit 512Mi → 256Mi; pods OOMKilled
// since." Several edits are listed likeliest culprit first. It names who
// made the change and when, as the sentence it replaces did.
func revisionDiffSentences(f caseFacts) []sentence {
	revision, edits := revisionEdits(f)
	if len(edits) == 0 {
		return nil
	}
	which := "the new revision"
	if revision > 0 {
		which = "revision " + strconv.Itoa(revision)
	}
	text := "Only change in " + which + ": " + editList(edits)
	if len(edits) > 1 {
		text = "Changes in " + which + ", likeliest culprit first: " +
			editList(edits)
	}
	if f.p.Cause != nil && f.p.Cause.Change != nil {
		text += changeBy(*f.p.Cause.Change)
	}
	if since := failedSince(f); since != "" {
		text += "; " + since
	}
	return []sentence{{part: partProof, weight: weightChange,
		text: endSentence(text)}}
}

// changeBy is " (by alice)" for the person who made the revision, and
// nothing when nobody is known: the lead already says when.
func changeBy(change inventory.Change) string {
	if who := person(change.Actor); who != "" {
		return " (by " + who + ")"
	}
	return ""
}

// failedSince says how the pods fail, from the findings' own reasons.
func failedSince(f caseFacts) string {
	for _, m := range f.members {
		switch m.Reason {
		case reasons.OOMKilled, reasons.OOMKILLED:
			return "pods OOMKilled since"
		case reasons.CrashLoopBackOff:
			return "pods crash-looping since"
		case reasons.ImagePullBackOff, reasons.ErrImagePull:
			return "pods cannot pull the image since"
		case reasons.ReleaseRegression:
			return "pods restarting more than before since"
		}
	}
	return ""
}

// hasRevisionEdits reports a note whose revision sentence already says
// what the blamed change did, so the change sentence would repeat it.
func hasRevisionEdits(f caseFacts) bool {
	_, edits := revisionEdits(f)
	return len(edits) > 0
}
