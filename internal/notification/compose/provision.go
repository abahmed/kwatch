package compose

// weightProvision puts the provisioner's own words above the
// scheduler's: they name the missing class.
const weightProvision = 0.6

// provisionSentences quote the provisioning failure of a claim whose
// StorageClass does not exist: the claim's event says which class it
// could not find.
func provisionSentences(f caseFacts) []sentence {
	if !causeRuleIs(f, "storage-class-missing",
		"storage-class-missing-pending") {
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
