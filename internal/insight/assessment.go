package insight

import (
	"sort"
	"strings"

	"github.com/abahmed/kwatch/internal/model"
)

const maxCauseCandidates = 8

func (e *Engine) finalizeAssessment(
	inc *model.Incident,
	ins *Insight,
) {
	if inc == nil || ins == nil {
		return
	}
	ins.Candidates = e.causeCandidates(inc, ins)
	if len(ins.Candidates) > 0 {
		top := ins.Candidates[0]
		ins.RootCause = top.Ref
		if top.Explanation != "" && top.Score > int(ins.Confidence*100) {
			ins.Cause = top.Explanation
			ins.Pattern = top.Pattern
			ins.Evidence = appendUniqueStrings(
				ins.Evidence, top.Supporting...,
			)
			ins.Confidence = min(float64(top.Score)/100, 0.99)
		}
		ins.Contradictions = append(
			ins.Contradictions,
			top.Contradicting...,
		)
	}
	switch {
	case ins.Cause == "":
		ins.CauseState = CauseUnknown
	case ins.Confidence >= 0.90 && len(ins.Evidence) > 0 &&
		len(ins.Contradictions) == 0:
		ins.CauseState = CauseConfirmed
	case ins.Confidence >= 0.65 && len(ins.Evidence) > 0:
		ins.CauseState = CauseLikely
	default:
		ins.CauseState = CauseUnknown
	}
}

func appendUniqueStrings(values []string, additions ...string) []string {
	seen := make(map[string]bool, len(values)+len(additions))
	for _, value := range values {
		seen[value] = true
	}
	for _, value := range additions {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		values = append(values, value)
	}
	return values
}

func (e *Engine) causeCandidates(
	inc *model.Incident,
	ins *Insight,
) []CauseCandidate {
	var candidates []CauseCandidate
	if ins.Cause != "" {
		candidates = append(candidates, CauseCandidate{
			Ref: inc.Ref(), Pattern: ins.Pattern,
			Explanation: ins.Cause,
			Score:       int(ins.Confidence * 100),
			Supporting:  append([]string(nil), ins.Evidence...),
		})
	}
	roots := e.rootCauses(inc)
	roots = e.dropUnchangedConfigRoots(roots)
	e.rankRootsByEvidence(roots)
	for _, root := range roots {
		ref := model.ObjectRef{
			Kind: root.Kind, Namespace: root.Namespace, Name: root.Name,
		}
		candidate := CauseCandidate{Ref: ref, Score: root.score + root.depth*10}
		if ref.Namespace != "" && ref.Namespace != inc.Namespace {
			candidate.Supporting = append(
				candidate.Supporting,
				"the graph links this cross-namespace infrastructure dependency",
			)
		}
		candidate.Explanation, candidate.Pattern = describeRootCauses(
			[]modelCauseRef{root},
		)
		if e.activeChecker != nil {
			if e.activeChecker(ref.Kind, ref.Namespace, ref.Name) {
				candidate.Score += 120
				candidate.Supporting = append(
					candidate.Supporting,
					"the dependency has an active incident",
				)
			} else {
				candidate.Score -= 40
				candidate.Contradicting = append(
					candidate.Contradicting,
					"the dependency has no active incident",
				)
			}
		}
		candidates = append(candidates, candidate)
	}
	candidates = mergeCandidates(candidates)
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})
	if len(candidates) > maxCauseCandidates {
		candidates = candidates[:maxCauseCandidates]
	}
	return candidates
}

func mergeCandidates(candidates []CauseCandidate) []CauseCandidate {
	byKey := make(map[string]int)
	merged := make([]CauseCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		key := candidate.Ref.Key() + "|" + candidate.Pattern
		if strings.TrimSpace(candidate.Ref.Name) == "" {
			key = "pattern|" + candidate.Pattern
		}
		if index, ok := byKey[key]; ok {
			if candidate.Score > merged[index].Score {
				merged[index].Score = candidate.Score
			}
			merged[index].Supporting = append(
				merged[index].Supporting, candidate.Supporting...,
			)
			merged[index].Contradicting = append(
				merged[index].Contradicting, candidate.Contradicting...,
			)
			continue
		}
		byKey[key] = len(merged)
		merged = append(merged, candidate)
	}
	return merged
}
