package dashboard

import (
	"sort"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/redact"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// maxCandidateProofs bounds the proofs listed for one candidate.
const maxCandidateProofs = 3

// maxBlastPods bounds the pods listed under a change.
const maxBlastPods = 50

// RoundCandidate is one possible cause with its score and its
// strongest proofs. State is "kept" or "ruled out" when a check
// separated the two, else "close".
type RoundCandidate struct {
	Cause  string       `json:"cause"`
	Score  int          `json:"score"`
	State  string       `json:"state"`
	Proofs []RoundProof `json:"proofs,omitempty"`
}

// RoundProof is one line of evidence for or against a candidate.
type RoundProof struct {
	Text     string `json:"text"`
	Supports bool   `json:"supports"`
}

// RoundPod is a failing pod and whether it uses the changed object.
type RoundPod struct {
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
	Read      bool   `json:"read"`
}

// candidatesOf lists the stated cause and its rival, best first; nil
// when the cause has no rival.
func candidatesOf(
	cause *rootcause.CauseRecord, check *RoundCheck,
) []RoundCandidate {
	if cause == nil || cause.Rival == nil {
		return nil
	}
	state, rivalState := "close", "close"
	if check != nil {
		state, rivalState = "kept", "ruled out"
	}
	out := []RoundCandidate{candidateOf(cause, state),
		candidateOf(cause.Rival, rivalState)}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Score > out[j].Score
	})
	return out
}

func candidateOf(c *rootcause.CauseRecord, state string) RoundCandidate {
	out := RoundCandidate{Cause: c.Root.String(), State: state,
		Score: int(c.Score*100 + 0.5)}
	for _, item := range c.Proof {
		if len(out.Proofs) == maxCandidateProofs {
			break
		}
		if item.Text != "" {
			out.Proofs = append(out.Proofs, RoundProof{
				Text: redact.Evidence(item.Text), Supports: item.Supports})
		}
	}
	return out
}

// blastPodsOf lists the incident's failing pods and whether each uses the
// cause's root, directly or through its owners. It is nil unless the
// counts agree with the overlap kwatch stated, so the list never
// contradicts the fraction beside it.
func blastPodsOf(
	model inventory.Reader, p incident.Incident, b *RoundBlast,
) []RoundPod {
	if model == nil || p.Cause == nil || b == nil {
		return nil
	}
	users := map[inventory.EntityID]bool{}
	for _, rel := range []inventory.RelationType{inventory.References,
		inventory.Mounts} {
		for _, id := range model.Related(p.Cause.Root, rel,
			inventory.Incoming) {
			users[id] = true
		}
	}
	seen := map[inventory.EntityID]bool{}
	var out []RoundPod
	read := 0
	for _, f := range sortedFindings(p) {
		pod, ok := rootcause.PodOf(model, f.Entity)
		if !ok || seen[pod] {
			continue
		}
		seen[pod] = true
		uses := users[pod]
		for _, owner := range rootcause.OwnerChain(model, pod) {
			uses = uses || users[owner]
		}
		if uses {
			read++
		}
		out = append(out, RoundPod{Namespace: pod.Namespace,
			Name: pod.Name, Read: uses})
	}
	if read != b.Count || len(out) != b.Total || len(out) > maxBlastPods {
		return nil
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Read && !out[j].Read
	})
	return out
}

// logRates is the errors per minute of the last log window and of the
// windows before it ("usual"); usual is 0 when nothing was before.
func logRates(
	model inventory.Reader, id inventory.EntityID, errors, window int,
) (now, usual float64) {
	if window > 0 {
		now = float64(errors) / (float64(window) / 60)
	}
	if model != nil {
		span := attrNumber(model, id, kube.AttrLogBaselineSpan)
		if span > 0 {
			usual = attrNumber(model, id, kube.AttrLogBaseline) /
				(span / 60)
		}
	}
	return round1(now), round1(usual)
}

func round1(v float64) float64 { return float64(int(v*10+0.5)) / 10 }
