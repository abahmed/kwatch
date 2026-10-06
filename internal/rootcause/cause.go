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
	// Began is when the cause went wrong, when known. An incident that
	// was announced long before a cause began keeps its root instead of
	// being taken over by it.
	Began time.Time `json:",omitempty"`
}

// Confidence levels shown to readers. They match explain's
// ConfidenceHigh and ConfidenceLikely, which say how they were
// calibrated.
const (
	High   = 0.70
	Likely = 0.5
)
