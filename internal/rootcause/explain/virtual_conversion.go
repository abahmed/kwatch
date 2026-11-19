package explain

import (
	"regexp"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// conversionPattern is the API server's answer to a read that needs a
// conversion its webhook cannot do: group 1 is the API group of the
// resource.
var conversionPattern = regexp.MustCompile(
	`conversion webhook for ([a-z0-9.-]+)/`)

// conversionHops lead from a failing pod or container whose error says a
// conversion webhook failed to the CustomResourceDefinitions of that API
// group that report a conversion backend that is gone or has no ready
// pods.
func (v *view) conversionHops(id inventory.EntityID) []hop {
	m := conversionPattern.FindStringSubmatch(v.text(id))
	if m == nil {
		return nil
	}
	var out []hop
	for _, crd := range v.s.Model.Entities(kube.KindCRD) {
		if !strings.HasSuffix(crd.Name, "."+m[1]) {
			continue
		}
		for _, f := range v.s.Findings[crd] {
			if f.Mode.Within(detection.ModeWebhookNoEndpoints) ||
				f.Mode.Within(detection.ModeWebhookBackendMissing) {
				out = append(out, hop{link: LinkUses, to: crd})
				break
			}
		}
	}
	return out
}

// conversionDownRow says a CustomResourceDefinition whose conversion
// webhook backend is gone explains the clients that fail on its
// conversions.
var conversionDownRow = Row{
	Name: RuleConversionWebhookDown,
	Cause: Side{Kind: kube.KindCRD, Modes: []detection.Mode{
		detection.ModeWebhookNoEndpoints,
		detection.ModeWebhookBackendMissing}},
	Link:   LinkUses,
	Effect: Side{Kind: kube.KindPod, Modes: callerFailures},
	Prior:  0.75,
}

// errorNamedHops are the hops read from what a claim's, pod's or
// container's own error names: the volume refused as bound elsewhere and
// the conversion webhook that failed.
func (v *view) errorNamedHops(id inventory.EntityID) []hop {
	var out []hop
	if id.Kind == kube.KindPVC || id.Kind == kube.KindPod {
		out = append(out, v.boundElsewhereHops(id)...)
	}
	if id.Kind == kube.KindContainer || id.Kind == kube.KindPod {
		out = append(out, v.conversionHops(id)...)
	}
	return out
}
