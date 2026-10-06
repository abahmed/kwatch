package rootcause

import "github.com/abahmed/kwatch/internal/inventory"

// Proof is one piece of evidence for or against a cause: one scorer's
// contribution (explain.Contribution) as an incident keeps it.
type Proof struct {
	// Code says what the evidence is. Writers word it from Code and the
	// numbers below; a proof without a code, such as one written by
	// hand, is shown as its Text.
	Code ProofCode `json:",omitempty"`
	// Count and Total are the numbers the code counts, as in "Count of
	// Total dependents fail". Codes without numbers leave them zero.
	Count int `json:",omitempty"`
	Total int `json:",omitempty"`
	// Fields are the field paths a ProofChanged change touched.
	Fields []string `json:",omitempty"`
	// Edits are what a ProofNewRevisionFails revision changed in its pod
	// template compared with the previous revision, likeliest culprit
	// first; Count is the revision number. Values are already redacted.
	Edits []inventory.FieldChange `json:",omitempty"`
	// Text is explain's own wording, for traces and audit.
	Text   string
	Weight float64
	// Supports is true for evidence for the cause, false against it.
	Supports bool
}

// ProofCode names one kind of evidence a scorer gives. The values are
// stable: incidents persist them.
type ProofCode string

// Proof codes, by the scorer that gives them.
const (
	// ProofBeganBefore: the cause began before the failures (temporal).
	ProofBeganBefore ProofCode = "began-before"
	// ProofBeganAfter: the cause began well after the failures.
	ProofBeganAfter ProofCode = "began-after"
	// ProofDependentsFail: Count of the cause's Total dependents fail
	// (coverage).
	ProofDependentsFail ProofCode = "dependents-fail"
	// ProofNoHealthyPeer: no healthy node outside a zone or pool to
	// compare with (exclusivity veto).
	ProofNoHealthyPeer ProofCode = "no-healthy-peer"
	// ProofSiblingsFail: replicas that do not depend on the cause fail
	// too (exclusivity).
	ProofSiblingsFail ProofCode = "siblings-fail"
	// ProofSiblingsHealthy: replicas that do not depend on the cause are
	// healthy (exclusivity).
	ProofSiblingsHealthy ProofCode = "siblings-healthy"
	// ProofNothingUpstream: nothing a workload blamed for itself depends
	// on explains its failures (exclusivity).
	ProofNothingUpstream ProofCode = "nothing-upstream"
	// ProofErrorsName: the errors of Count of Total failures name the
	// cause (specificity).
	ProofErrorsName ProofCode = "errors-name"
	// ProofCreated: the cause was created shortly before (change).
	ProofCreated ProofCode = "created"
	// ProofChanged: the cause changed shortly before; Fields lists what
	// changed, when known (change).
	ProofChanged ProofCode = "changed"
	// ProofRevertRecovered: reverting the cause's change brought its
	// dependents back (outcome).
	ProofRevertRecovered ProofCode = "revert-recovered"
	// ProofFailedAfterChange: the dependents failed soon after the cause
	// changed (outcome).
	ProofFailedAfterChange ProofCode = "failed-after-change"
	// ProofNewRevisionFails: only the new revision of a rollout fails
	// (revision).
	ProofNewRevisionFails ProofCode = "new-revision-fails"
	// ProofEveryRevisionFails: every revision of a rollout fails alike
	// (revision).
	ProofEveryRevisionFails ProofCode = "every-revision-fails"
	// ProofWorkloadsMeet: the failures of Count workloads meet at the
	// cause (shared).
	ProofWorkloadsMeet ProofCode = "workloads-meet"
	// ProofOneWorkload: only one workload fails behind a shared cause
	// (shared).
	ProofOneWorkload ProofCode = "one-workload"
	// ProofBaseline: the cause deviates Count percent from its learned
	// baseline (baseline).
	ProofBaseline ProofCode = "baseline"
	// ProofDoubtfulData: some of the data is doubtful; Text says what
	// (quality).
	ProofDoubtfulData ProofCode = "doubtful-data"
)
