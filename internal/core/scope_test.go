package core

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/problem"
	"github.com/abahmed/kwatch/internal/reason"
	"github.com/abahmed/kwatch/internal/signal"
)

type namespaceScope string

func (s namespaceScope) Allows(_ knowledge.Reader, sig signal.Signal) bool {
	return sig.Entity.Namespace == string(s)
}

func scopedProblem(memberNS, rootNS string) problem.Problem {
	member := signal.Signal{Entity: knowledge.EntityID{
		Kind: "pod", Namespace: memberNS, Name: "p"}, Reason: "Error"}
	root := signal.Signal{Entity: knowledge.EntityID{
		Kind: "pvc", Namespace: rootNS, Name: "data"}, Reason: "Pending"}
	return problem.Problem{
		Members: map[signal.Key]signal.Signal{member.Key(): member},
		Cause:   &reason.Hypothesis{RootSignals: []signal.Signal{root}},
	}
}

func TestProblemScopeMatchesAnyMemberOrRootSignal(t *testing.T) {
	inScope := ProblemScope(nil, namespaceScope("shop"))

	assert.True(t, inScope(scopedProblem("shop", "other")))
	assert.True(t, inScope(scopedProblem("other", "shop")))
	assert.False(t, inScope(scopedProblem("other", "other")))
	assert.False(t, inScope(problem.Problem{}))
}

func TestEngineDropsOutOfScopeDecisions(t *testing.T) {
	e := &Engine{deps: Dependencies{
		InScope: ProblemScope(nil, namespaceScope("shop")),
	}}
	decisions := []problem.Decision{
		{Problem: scopedProblem("shop", "")},
		{Problem: scopedProblem("dev", "")},
	}

	kept := e.inScope(decisions)

	assert.Len(t, kept, 1)
	e.deps.InScope = nil
	assert.Len(t, e.inScope(decisions), 2)
}
