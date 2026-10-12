package explain

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
)

// pullNoteReasons are the kubelet event reasons of an image pull. A
// mount, sandbox or probe event of the same pod is not about its pull.
var pullNoteReasons = map[string]bool{
	"Failed": true, "BackOff": true, "ErrImagePull": true,
	"ImagePullBackOff": true, "InspectFailed": true,
}

// pullText is the text of effect's failed pulls only: its pull findings
// and the pull notes of it and its pod. A registry is judged by what its
// pulls said, never by an unrelated event of the same pod ("timed out
// waiting for the condition" from a mount is no network fault of the
// registry). An image the kubelet may not pull (pull policy Never)
// never reached a registry, so it reports no text.
func (v *view) pullText(effect inventory.EntityID) (string, bool) {
	var parts []string
	for _, f := range v.s.Findings[effect] {
		if f.Mode.Within(detection.ModeImagePullNeverPull) {
			return "", false
		}
		if !f.Mode.Within(detection.ModeImagePull) {
			continue
		}
		parts = append(parts, f.Summary)
		for _, e := range f.Evidence {
			parts = append(parts, e.Value)
		}
	}
	sources := []inventory.EntityID{effect}
	if pod, ok := v.podOf(effect); ok && pod != effect {
		sources = append(sources, pod)
	}
	for _, source := range sources {
		for _, note := range v.s.Model.Notes(source, v.since) {
			if pullNoteReasons[note.Reason] {
				parts = append(parts, note.Message)
			}
		}
	}
	return strings.Join(parts, " "), true
}
