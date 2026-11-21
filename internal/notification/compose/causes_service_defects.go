package compose

// Wording and category of the rows that blame a Service whose own
// declaration cannot carry the call.
func init() {
	for rule, w := range map[string]causeWording{
		"service-protocol-mismatch": {
			words: "exposes the port over UDP, but its callers use TCP"},
		"service-name-is-address": {
			words: "is an external name set to an IP address, which does " +
				"not resolve"},
		"service-family-mismatch": {
			words: "has an IP family its pods have no address of"},
	} {
		causeWords[rule] = w
		causeCategories[rule] = categoryDependency
	}
}
