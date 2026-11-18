package explain

import (
	"sort"

	"github.com/abahmed/kwatch/internal/rootcause"
)

func init() {
	scorers = append(scorers, scorer{"address-not-stated",
		scoreAddressNotStated})
}

// scoreAddressNotStated notes the workloads an endpoint explains whose
// errors only say what kind of service they cannot reach: the same
// failure, address not stated. It weighs nothing; the workloads that
// name the address carry the cause.
func scoreAddressNotStated(v *view, c *candidate) outcome {
	if c.id.Kind != KindExternalEndpoint {
		return outcome{}
	}
	named := map[string]bool{}
	for _, effect := range c.others() {
		if _, says := v.callOf(effect); says {
			continue
		}
		if endpoint, ok := v.kindOnlyEndpoint(effect); ok &&
			endpoint == c.id.Name {
			named[v.workloadOf(effect).Name] = true
		}
	}
	if len(named) == 0 {
		return outcome{}
	}
	names := make([]string, 0, len(named))
	for name := range named {
		names = append(names, name)
	}
	sort.Strings(names)
	return outcome{code: rootcause.ProofAddressNotStated,
		count: len(names), fields: names,
		text: "some workloads fail the same way without stating the address"}
}
