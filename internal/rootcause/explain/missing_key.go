package explain

import (
	"regexp"
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// missingKeyPattern is the kubelet's own sentence for a key a container
// reads from a Secret or ConfigMap that does not hold it: "couldn't
// find key db-password in Secret shop/db-creds". It names the object
// by namespace and name.
var missingKeyPattern = regexp.MustCompile(
	`couldn't find key \S+ in (secret|configmap) ([^\s/"]+)/([^\s"]+)`)

// kubeletNames reports whether the kubelet named object as lacking a
// key a container of pod reads. The object exists and so has no finding
// of its own; the kubelet's sentence is the evidence that it is what is
// wrong, whenever it was last edited.
func (v *view) kubeletNames(pod, object inventory.EntityID) bool {
	if pod.Kind != kube.KindPod ||
		(object.Kind != kube.KindSecret &&
			object.Kind != kube.KindConfigMap) {
		return false
	}
	text := strings.ToLower(v.unitErrorText(pod))
	for _, m := range missingKeyPattern.FindAllStringSubmatch(text, -1) {
		name := strings.TrimRight(m[3], ".,;:)")
		if m[1] == string(object.Kind) &&
			m[2] == strings.ToLower(object.Namespace) &&
			name == strings.ToLower(object.Name) {
			return true
		}
	}
	return false
}

// keyMissingModes is ModeKeyMissing for a Secret or ConfigMap the
// kubelet named as lacking a key the effect's pod reads.
func (v *view) keyMissingModes(
	id, effect inventory.EntityID, link LinkType,
) []modeHealth {
	pod, ok := v.podOf(effect)
	if link != LinkUses || !ok || !v.kubeletNames(pod, id) {
		return nil
	}
	return pseudoFailing(ModeKeyMissing)
}
