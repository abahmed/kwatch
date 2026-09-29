package core

import (
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/problem"
	"github.com/abahmed/kwatch/internal/signal"
)

// SignalScope decides whether one signal is in scope.
type SignalScope interface {
	Allows(model knowledge.Reader, sig signal.Signal) bool
}

// ProblemScope returns an InScope function: a problem is in scope when
// any signal it explains, or any signal of its root, is.
func ProblemScope(
	model knowledge.Reader, scope SignalScope,
) func(problem.Problem) bool {
	return func(p problem.Problem) bool {
		for _, member := range p.Members {
			if scope.Allows(model, member) {
				return true
			}
		}
		if p.Cause == nil {
			return false
		}
		for _, root := range p.Cause.RootSignals {
			if scope.Allows(model, root) {
				return true
			}
		}
		return false
	}
}
