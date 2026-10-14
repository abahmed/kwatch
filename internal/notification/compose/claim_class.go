package compose

// weightClaimClass ranks the missing storage class above the generic
// facts of a claim that waits for a volume.
const weightClaimClass = 0.7

// claimClassSentences say why a claim that names no StorageClass waits:
// "Claim cache-0 sets no storage class and the cluster has no default
// one." The row that blames it checked the classes and static volumes.
func claimClassSentences(f caseFacts) []sentence {
	if !causeRuleIs(f, "claim-no-storage-class") {
		return nil
	}
	return []sentence{{part: partProof, weight: weightClaimClass,
		text: "Claim " + f.p.Cause.Root.Name + " sets no storage class " +
			"and the cluster has no default one."}}
}
