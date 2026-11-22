package compose

import "github.com/abahmed/kwatch/internal/rootcause/explain"

// Wording and category of the rows that blame a Service whose own
// declaration cannot carry the call, or a policy that isolates it.
func init() {
	for rule, w := range map[string]causeWording{
		"service-protocol-mismatch": {
			words: "exposes the port over UDP, but its callers use TCP"},
		"service-name-is-address": {
			words: "is an external name set to an IP address, which does " +
				"not resolve"},
		"service-family-mismatch": {
			words: "uses an IP family its pods have no address in"},
		explain.RuleFinalizerHoldsAddresses: {
			words: "holds Terminating pods that keep their addresses in use"},
		"policy-isolates-backend": {
			words: "cuts off every pod of the service the API server calls"},
	} {
		causeWords[rule] = w
		causeCategories[rule] = categoryDependency
		if rule == explain.RuleFinalizerHoldsAddresses {
			causeCategories[rule] = categoryFinalizer
		}
	}
}
