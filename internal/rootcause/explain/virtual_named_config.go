package explain

import (
	"regexp"
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// maxNamedConfigs bounds the ConfigMaps one error can lead to.
const maxNamedConfigs = 4

// objectRef finds "namespace/name" in lower-case error text.
var objectRef = regexp.MustCompile(
	`\b([a-z0-9](?:[a-z0-9-]*[a-z0-9])?)/` +
		`([a-z0-9](?:[a-z0-9.-]*[a-z0-9])?)\b`)

// namedConfigHops lead from a failing pod or container whose error
// names an existing ConfigMap as namespace/name to that ConfigMap. A
// program that reads a ConfigMap through the API names it in its own
// errors, though its pod spec never mounts it. The ConfigMap explains
// the error only when it changed (the config rows decide).
func (v *view) namedConfigHops(id inventory.EntityID) []hop {
	pod, ok := v.unitOf(id)
	if !ok || !v.unitGate(pod) {
		return nil
	}
	var out []inventory.EntityID
	for _, f := range v.unitFindingsIn(pod, callerFailures) {
		for _, text := range v.evidenceTexts(f) {
			for _, m := range objectRef.FindAllStringSubmatch(
				strings.ToLower(text), -1) {
				cm := inventory.CoreID(kube.KindConfigMap, m[1], m[2])
				v.gate(pod, cm)
				if v.s.Model.Exists(cm) && !containsID(out, cm) &&
					len(out) < maxNamedConfigs {
					out = append(out, cm)
				}
			}
		}
	}
	return hopsTo(LinkUses, out...)
}
