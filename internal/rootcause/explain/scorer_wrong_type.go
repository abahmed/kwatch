package explain

import "github.com/abahmed/kwatch/internal/rootcause"

func init() {
	scorers = append(scorers, scorer{"wrong-type", scoreWrongType})
}

// wrongTypeWeight is what a pull Secret of the wrong type adds: the
// registry saw no credentials because the Secret holds none, which
// settles the refusal. It outweighs the release that named the Secret,
// the one other change that fits the failure.
const wrongTypeWeight = 0.35

// scoreWrongType supports a Secret the failing pods name as their image
// pull Secret when it is not a registry login and the registry refused
// the pull as anonymous: the kubelet read nothing from it.
func scoreWrongType(v *view, c *candidate) outcome {
	for _, how := range c.covers {
		if how.match.causeMode == ModeWrongType {
			return outcome{weight: wrongTypeWeight,
				code: rootcause.ProofErrorFits,
				text: "the pods pull with it, it holds no registry " +
					"login, and the registry refused the pull"}
		}
	}
	return outcome{}
}
