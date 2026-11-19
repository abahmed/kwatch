package compose

import "github.com/abahmed/kwatch/internal/inventory/kube"

// conversionConsequence says what a CustomResourceDefinition whose
// conversion webhook backend is gone breaks. A conversion webhook is not
// an admission webhook: nothing is rejected, but every read that needs a
// conversion fails.
func conversionConsequence(f caseFacts, webhook bool) ([]sentence, bool) {
	if !webhook || f.lead.Entity.Kind != kube.KindCRD {
		return nil, false
	}
	return []sentence{{part: partConsequence, weight: 1,
		text: "Reads of its resources that need a conversion fail."}}, true
}
