package explain

import (
	"sort"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// sharedError is the error a failure-signature root stands for, as one of
// the failing pods wrote it: the first "error" evidence, in entity order,
// that normalises to the root's signature. The signature itself holds only
// normalised words, which no pod wrote. Empty for any other root.
func (s Snapshot) sharedError(c Cause) string {
	if c.Root.Kind != KindFailureSignature {
		return ""
	}
	ids := make([]inventory.EntityID, 0, len(s.Findings))
	for id := range s.Findings {
		if id.Kind == kube.KindPod || id.Kind == kube.KindContainer {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool {
		return ids[i].String() < ids[j].String()
	})
	for _, id := range ids {
		for _, f := range s.Findings[id] {
			if text := signatureEvidence(f, c.Root.Name); text != "" {
				return text
			}
		}
	}
	return ""
}

// signatureEvidence is the finding's own error text when it normalises to
// the signature name.
func signatureEvidence(f detection.Finding, name string) string {
	for _, e := range f.Evidence {
		if e.Label != "error" {
			continue
		}
		if signature := normalizeSignature(e.Value); signature != "" &&
			signatureName(f.Mode, signature) == name {
			return e.Value
		}
	}
	return ""
}
