package rootcause

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
)

// KindScheduling is the virtual entity for a cluster-wide scheduling
// constraint, named after the dominant blocker ("Insufficient memory").
const KindScheduling inventory.Kind = "scheduling"

// CauseRecord is the record of a stated root cause: what an incident
// keeps and messages are written from. explain.Snapshot.Record builds it
// from an explain.Cause. Writers read its fields, never its text: Mode,
// Rule and each Proof's Code say what to write, while Summary and
// Proof.Text are explain's own words for traces and audit.
type CauseRecord struct {
	// Rule names the propagation row that links the root to its
	// failures. A record without a rule was not built by explain.
	Rule string
	// Mode is the root's mode that matched the row, such as
	// "VolumeFull" or the pseudo mode "Webhook.Timeout". Empty when
	// the row matched any mode.
	Mode detection.Mode `json:",omitempty"`
	// Root is the entity blamed for the symptom.
	Root inventory.EntityID
	// RootFindings are the root's own active findings, if any.
	RootFindings []detection.Finding
	// Change is the change blamed, when the cause is a modification.
	Change *inventory.Change
	// RollbackRevision is the revision a rollback of a blamed rollout
	// returns to, when it is known.
	RollbackRevision string
	// Chain is the path from the root to the symptom's entity.
	Chain []inventory.EntityID
	// Summary states the cause in one sentence, in explain's words.
	Summary string
	// Proof is the evidence for and against the cause, strongest first.
	// It is persisted under its first name, Points.
	Proof []Proof `json:"Points"`
	// Score is the confidence, between 0 and 1.
	Score float64
	// Unverified names, in plain words, what kwatch could not see where
	// a cause might have been ("secrets in billing").
	Unverified []string `json:",omitempty"`
	// Rival is another cause for the same failures whose score is
	// within RivalMargin of this one's. While it is set the cause is
	// not stated alone: a message names both. It never has a Rival.
	Rival *CauseRecord `json:",omitempty"`
	// Checked says, in explain's words, what counterfactual checks
	// compared the failures with: "node n3 is healthy for 12 other
	// pods". Strongest first, empty when nothing was compared.
	Checked []string `json:",omitempty"`
	// Began is when the cause went wrong, when known. An incident that
	// was announced long before a cause began keeps its root instead of
	// being taken over by it.
	Began time.Time `json:",omitempty"`
	// Hops is the failure chain from the root to the deepest failure
	// the cause was extended to, the root first; empty when the cause
	// explains only what it reaches directly.
	Hops []Hop `json:",omitempty"`
	// Beyond are failures one step past the chain's limit: they follow
	// from it but are not claimed by it.
	Beyond []Hop `json:",omitempty"`
	// Logins are the registry logins the failing pods pull with, when
	// the root is a registry that refuses logins.
	Logins []PullLogin `json:",omitempty"`
	// Refused is the registry's own answer, as the pull error says it,
	// when the error holds one: "unauthorized: authentication required".
	Refused string `json:",omitempty"`
	// ServingCert is the TLS Secret a webhook's pods serve when its
	// certificate has already expired, for a webhook whose calls fail
	// the TLS handshake.
	ServingCert *ServingCert `json:",omitempty"`
}

// Hop is one step of a failure chain: an entity that failed, a workload,
// a Service or a route, and when it began to.
type Hop struct {
	Entity inventory.EntityID
	Began  time.Time `json:",omitempty"`
}

// Confidence levels shown to readers. They match explain's
// ConfidenceHigh and ConfidenceLikely, which say how they were
// calibrated.
const (
	High   = 0.70
	Likely = 0.5
	// RivalMargin is how close two causes of the same failures must
	// score to be told as two possible causes instead of one answer.
	RivalMargin = 0.05
)
