package compose

import "github.com/abahmed/kwatch/internal/rootcause/explain"

// weightProvision puts the provisioner's own words above the
// scheduler's: they name the missing class.
const weightProvision = 0.6

// provisionSentences quote the provisioning failure of a claim whose
// StorageClass does not exist: the claim's event says which class it
// could not find.
func provisionSentences(f caseFacts) []sentence {
	if !causeRuleIs(f, explain.RuleStorageClassMissing,
		explain.RuleStorageClassMissingPending) {
		return nil
	}
	for _, s := range f.members {
		if said := evidence(s, "event"); said != "" {
			return []sentence{{part: partProof, weight: weightProvision,
				text: "The provisioner says " + quoted(said) + "."}}
		}
	}
	return nil
}
